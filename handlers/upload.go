package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/chai2010/webp"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	xwebp "golang.org/x/image/webp"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

// WebP is explicitly decoded with the streaming x/image/webp decoder below.
// The WebP encoder dependency also registers a decoder that buffers the
// entire compressed file; never use image.Decode for WebP uploads.
var uploadSem = make(chan struct{}, config.DefaultMaxConcurrentUploads)

func InitUploadSemaphore(n int) {
	if n <= 0 || n > config.MaxConcurrentUploadsLimit {
		n = config.DefaultMaxConcurrentUploads
	}
	uploadSem = make(chan struct{}, n)
}

var (
	errMediaTooLarge   = errors.New("media exceeds upload limit")
	errImageBudgetBusy = errors.New("image processing capacity reached")
	decodedPixels      = struct {
		sync.Mutex
		inFlight int64
	}{}
)

// Enforce a shared memory budget for uploads AND preview regeneration. The
// upload semaphore alone allows many near-limit images to decode at once.
func reserveDecodedPixels(pixels int64) (func(), error) {
	if pixels <= 0 || pixels > config.MaxDecodedPixelsInFlight {
		return nil, errImageBudgetBusy
	}
	decodedPixels.Lock()
	if decodedPixels.inFlight+pixels > config.MaxDecodedPixelsInFlight {
		decodedPixels.Unlock()
		return nil, errImageBudgetBusy
	}
	decodedPixels.inFlight += pixels
	decodedPixels.Unlock()
	return func() {
		decodedPixels.Lock()
		decodedPixels.inFlight -= pixels
		decodedPixels.Unlock()
	}, nil
}

// copyFile writes to a temporary sibling and renames it into place. io.Copy
// can use optimized file-to-file copies; a bounded reader still protects
// against a local source growing after its initial size check.
func copyFile(dst string, src io.Reader, limit int64) error {
	out, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*"+filepath.Ext(dst))
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	n, copyErr := io.Copy(out, io.LimitReader(src, limit+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n > limit {
		return errMediaTooLarge
	}
	return os.Rename(tmp, dst)
}

var mimeToExt = map[string]string{
	"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif",
	"image/webp": "webp", "image/bmp": "bmp", "image/tiff": "tiff",
	"video/mp4": "mp4", "video/webm": "webm",
}

func isVideo(ext string) bool { return ext == "mp4" || ext == "webm" }

func storedExt(ext string, lossless bool) string {
	if !lossless && (ext == "bmp" || ext == "tiff") {
		return "jpg"
	}
	return ext
}

func canUseLosslessMode() bool {
	return config.Current.Compression.Quality == 100 && config.Current.Compression.Scale == 100
}

// inspectMediaFile validates a bounded file's type using magic bytes, not a
// user-provided extension or Content-Type. The reader is reset on return.
func inspectMediaFile(r io.ReadSeeker, name string, size, maxBytes int64) (string, error) {
	if size > maxBytes {
		return "", errMediaTooLarge
	}
	if size < 16 {
		return "", errors.New("file too small")
	}
	head := make([]byte, 512)
	n, err := r.Read(head)
	if err != nil && err != io.EOF {
		return "", err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	head = head[:n]
	// ISO-BMFF/MP4: accept ANY ftyp brand before consulting DetectContentType.
	// The WHATWG sniffer in net/http only matches ftyp boxes containing an
	// "mp4*" brand, so real-world camera/phone videos (isom/iso2/avc1/M4V...)
	// were rejected — especially URL downloads, which land in a nameless
	// temp file with no extension to fall back to.
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		return "mp4", utils.ValidateFileType(head, "mp4")
	}
	ext, ok := mimeToExt[http.DetectContentType(head)]
	if !ok {
		// TIFF/BMP/WebM are not detected on some Go versions. Fall back to
		// a supported extension, then demand its exact magic bytes below.
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
		if ext == "jpeg" {
			ext = "jpg"
		} else if ext == "tif" {
			ext = "tiff"
		}
	}
	if err := utils.ValidateFileType(head, ext); err != nil {
		return "", err
	}
	return ext, nil
}

// checkImageDimensions runs before any full image decode, including lossless
// uploads and thumbnail regeneration. A width/height cap alone still allows
// a 16k x 16k image to allocate >1 GB per decoded copy.
func checkImageDimensions(r io.ReadSeeker, ext string) (int64, error) {
	var cfg image.Config
	var err error
	if ext == "webp" {
		cfg, err = xwebp.DecodeConfig(r)
	} else {
		cfg, _, err = image.DecodeConfig(r)
	}
	if err != nil {
		return 0, fmt.Errorf("could not read image config: %w", err)
	}
	pixels := int64(cfg.Width) * int64(cfg.Height)
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > config.MaxImageDimension ||
		cfg.Height > config.MaxImageDimension || pixels > config.MaxImagePixels {
		return 0, fmt.Errorf("image %dx%d exceeds limits", cfg.Width, cfg.Height)
	}
	return pixels, nil
}

// The returned release function must be called after all resizing/encoding
// is finished, even on failure. Reserve BEFORE allocating a decoded image.
func decodeImage(r io.ReadSeeker, ext string) (image.Image, func(), error) {
	pixels, err := checkImageDimensions(r, ext)
	if err != nil {
		return nil, nil, err
	}
	release, err := reserveDecodedPixels(pixels)
	if err != nil {
		return nil, nil, err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		release()
		return nil, nil, err
	}
	var img image.Image
	if ext == "webp" {
		img, err = xwebp.Decode(r)
	} else {
		img, _, err = image.Decode(r)
	}
	if err != nil {
		release()
		return nil, nil, fmt.Errorf("invalid image: %w", err)
	}
	return img, release, nil
}

func thumbnail(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	scale := min(float64(maxW)/float64(b.Dx()), float64(maxH)/float64(b.Dy()))
	if scale >= 1 {
		return src
	}
	w := max(1, int(float64(b.Dx())*scale))
	h := max(1, int(float64(b.Dy())*scale))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func scaleImage(src image.Image, scalePercent int) image.Image {
	if scalePercent >= 100 {
		return src
	}
	b := src.Bounds()
	scale := float64(scalePercent) / 100.0
	w := max(1, int(float64(b.Dx())*scale))
	h := max(1, int(float64(b.Dy())*scale))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func stagePath(dir, ext string) (string, error) {
	f, err := os.CreateTemp(dir, ".upload-*."+ext)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// publishStaged atomically replaces a destination while retaining a hardlink
// to its previous contents. On a failed metadata commit, Rollback restores
// the previous file even if the extension/path was unchanged. Hardlinks are
// constant-time; the copy fallback supports filesystems without hardlinks.
type publishedFile struct{ dst, backup string }

func publishStaged(stage, dst string, maxBytes int64) (publishedFile, error) {
	p := publishedFile{dst: dst}
	fi, err := os.Lstat(dst)
	if err == nil {
		if !fi.Mode().IsRegular() {
			return p, errors.New("existing media is not a regular file")
		}
		p.backup, err = stagePath(filepath.Dir(dst), filepath.Ext(dst)[1:])
		if err != nil {
			return p, err
		}
		os.Remove(p.backup)
		if err = os.Link(dst, p.backup); err != nil {
			f, openErr := storage.OpenMedia(dst)
			if openErr != nil {
				return p, openErr
			}
			err = copyFile(p.backup, f, maxBytes)
			f.Close()
			if err != nil {
				os.Remove(p.backup)
				return p, err
			}
		}
	} else if !os.IsNotExist(err) {
		return p, err
	}
	if err := os.Rename(stage, dst); err != nil {
		if p.backup != "" {
			os.Remove(p.backup)
		}
		return p, err
	}
	return p, nil
}

func (p publishedFile) rollback() {
	if p.backup != "" {
		if err := os.Rename(p.backup, p.dst); err != nil {
			log.Printf("Critical: could not restore media %s: %v", p.dst, err)
		}
	} else if err := os.Remove(p.dst); err != nil && !os.IsNotExist(err) {
		log.Printf("Error removing failed upload %s: %v", p.dst, err)
	}
}

func (p publishedFile) finish() {
	if p.backup != "" {
		if err := os.Remove(p.backup); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing media backup %s: %v", p.backup, err)
		}
	}
}

func Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	select {
	case uploadSem <- struct{}{}:
		defer func() { <-uploadSem }()
	default:
		http.Error(w, "Too many concurrent uploads", http.StatusTooManyRequests)
		return
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	maxRequest := maxBytes + (1 << 20) // multipart headers and form fields
	if r.ContentLength > maxRequest {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequest)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		} else {
			http.Error(w, "Invalid multipart form", http.StatusBadRequest)
		}
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	name := r.FormValue("linkName")
	if !isValidLinkName(name) {
		http.Error(w, "Invalid link name", http.StatusBadRequest)
		return
	}
	urlStr := r.FormValue("url")
	if len(urlStr) > 2048 {
		http.Error(w, "URL too long", http.StatusBadRequest)
		return
	}
	unlock := storage.LockLinks(name)
	defer unlock()
	prev, exists := storage.Global.Get(name)
	if !exists {
		http.Error(w, "Link does not exist", http.StatusBadRequest)
		return
	}
	if err := os.MkdirAll(config.MediaDir, 0755); err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(config.PreviewDir, 0755); err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}

	var source io.ReadSeeker
	var sourceSize int64
	var sourceName string
	var localPath bool
	if urlStr != "" {
		u, err := url.Parse(urlStr)
		if err != nil {
			http.Error(w, "Invalid URL", http.StatusBadRequest)
			return
		}
		if u.Scheme == "http" || u.Scheme == "https" {
			f, size, err := downloadToTemp(r.Context(), urlStr, maxBytes)
			if err != nil {
				log.Printf("Download rejected: %v", err)
				http.Error(w, "Failed to load media", http.StatusBadRequest)
				return
			}
			defer func() { f.Close(); os.Remove(f.Name()) }()
			source, sourceSize, sourceName = f, size, u.Path
		} else {
			if u.IsAbs() || strings.HasPrefix(urlStr, "//") || !utils.IsValidLocalPath(urlStr) ||
				!config.AllowedMediaExts[strings.ToLower(filepath.Ext(urlStr))] {
				http.Error(w, "Invalid local media path", http.StatusBadRequest)
				return
			}
			f, err := utils.OpenExternalFile(config.Current.ExternalImageDir, urlStr)
			if err != nil {
				http.Error(w, "Path outside allowed directory or file unavailable", http.StatusForbidden)
				return
			}
			defer f.Close()
			fi, err := f.Stat()
			if err != nil {
				http.Error(w, "File unavailable", http.StatusBadRequest)
				return
			}
			source, sourceSize, sourceName, localPath = f, fi.Size(), urlStr, true
		}
	} else {
		f, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer f.Close()
		source, sourceSize, sourceName = f, header.Size, header.Filename
	}

	ext, err := inspectMediaFile(source, sourceName, sourceSize, maxBytes)
	if err != nil {
		if errors.Is(err, errMediaTooLarge) {
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		} else {
			log.Printf("Rejected media: %v", err)
			http.Error(w, "Invalid or unsupported media file", http.StatusBadRequest)
		}
		return
	}
	if localPath {
		nameExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(sourceName)), ".")
		if isVideo(ext) != (nameExt == "mp4" || nameExt == "webm") {
			http.Error(w, "Media type does not match local file", http.StatusBadRequest)
			return
		}
	}

	video := isVideo(ext)
	lossless := !video && canUseLosslessMode()
	saveExt := storedExt(ext, lossless)
	imageStage, err := stagePath(config.MediaDir, saveExt)
	if err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}
	defer os.Remove(imageStage)
	previewStage := ""
	if video {
		if err := copyFile(imageStage, source, maxBytes); err != nil {
			writeUploadError(w, err)
			return
		}
	} else {
		// Even in lossless mode, fully decode before storing: malformed files
		// must not be published just because their first 512 bytes look valid.
		img, release, err := decodeImage(source, ext)
		if errors.Is(err, errImageBudgetBusy) {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "Image processing busy; retry shortly", http.StatusTooManyRequests)
			return
		}
		if err != nil {
			log.Printf("Rejected image: %v", err)
			http.Error(w, "Invalid image or dimensions too large", http.StatusBadRequest)
			return
		}
		defer release()
		if !lossless {
			img = scaleImage(img, config.Current.Compression.Scale)
		}
		previewStage, err = stagePath(config.PreviewDir, "webp")
		if err != nil {
			writeUploadError(w, err)
			return
		}
		defer os.Remove(previewStage)
		if err := saveImage(thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", previewStage); err != nil {
			writeUploadError(w, err)
			return
		}
		if lossless {
			if _, err := source.Seek(0, io.SeekStart); err != nil {
				writeUploadError(w, err)
				return
			}
			err = copyFile(imageStage, source, maxBytes)
		} else {
			err = saveImage(img, saveExt, imageStage)
		}
		if err != nil {
			writeUploadError(w, err)
			return
		}
	}

	fi, err := os.Stat(imageStage)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	imagePath := storage.MediaPath(name, saveExt)
	previewPath := ""
	previewURL := ""
	if !video {
		previewPath = storage.PreviewFilePath(name)
		previewURL = "/api/preview/" + name
	}
	imagePub, err := publishStaged(imageStage, imagePath, maxBytes)
	if err != nil {
		writeUploadError(w, err)
		return
	}
	var previewPub publishedFile
	if previewStage != "" {
		previewPub, err = publishStaged(previewStage, previewPath, maxBytes)
		if err != nil {
			imagePub.rollback()
			writeUploadError(w, err)
			return
		}
	}

	newWP := *prev
	newWP.ID, newWP.LinkName = name, name
	newWP.ImageURL = "/" + name
	newWP.Preview = previewURL
	newWP.HasImage = true
	newWP.MIMEType = saveExt
	newWP.SizeBytes = fi.Size()
	newWP.ModTime = fi.ModTime().Unix()
	newWP.ImagePath = imagePath
	newWP.PreviewPath = previewPath
	if newWP.CreatedAt == 0 {
		newWP.CreatedAt = fi.ModTime().Unix()
	}
	updated, err := storage.Global.Update(name, func(wp *storage.Wallpaper) error {
		*wp = newWP
		return nil
	})
	if err != nil {
		if previewStage != "" {
			previewPub.rollback()
		}
		imagePub.rollback()
		writeUploadError(w, err)
		return
	}
	imagePub.finish()
	if previewStage != "" {
		previewPub.finish()
	}
	// Cleanup of the previous extension/preview only after commit. In
	// particular, a video replacing an image must not leave a stale preview.
	if prev.HasImage {
		if prev.ImagePath != imagePath {
			removeFiles(prev.ImagePath, "")
		}
		if prev.PreviewPath != previewPath {
			removeFiles("", prev.PreviewPath)
		}
	}
	storage.SchedulePrune(config.Current.MaxImages)
	log.Printf("Uploaded: %s (%s, %d KB)", name, saveExt, fi.Size()/1024)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(updated))
}

func writeUploadError(w http.ResponseWriter, err error) {
	log.Printf("Upload failed: %v", err)
	if errors.Is(err, errMediaTooLarge) {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
	} else {
		http.Error(w, "Failed to save media", http.StatusInternalServerError)
	}
}

// saveImage encodes to a temporary sibling; a failed encode cannot truncate
// either an existing stage file or a previously published image.
func saveImage(img image.Image, format, path string) error {
	out, err := os.CreateTemp(filepath.Dir(path), ".tmp-*"+filepath.Ext(path))
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	encodeErr := encodeImage(out, img, format)
	closeErr := out.Close()
	if encodeErr != nil {
		return encodeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, path)
}

func encodeImage(w io.Writer, img image.Image, format string) error {
	quality := config.Current.Compression.Quality
	switch format {
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(w, img)
	case "gif":
		return gif.Encode(w, img, &gif.Options{NumColors: config.GIFColors})
	case "webp":
		return webp.Encode(w, img, &webp.Options{Quality: float32(quality)})
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}
