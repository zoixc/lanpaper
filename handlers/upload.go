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
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	_ "golang.org/x/image/tiff"
	xwebp "golang.org/x/image/webp"

	"lanpaper/config"
	"lanpaper/middleware"
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
		idle := decodedPixels.inFlight == 0
		decodedPixels.Unlock()
		// A large decode leaves tens of MB of garbage that the runtime would
		// keep resident for minutes. Hand it back once no image work is left.
		if idle && pixels >= freeOSMemoryMinPixels {
			go debug.FreeOSMemory()
		}
	}, nil
}

// freeOSMemoryMinPixels is the decode size (~4 MP) from which returning
// memory to the OS is worth a forced GC cycle.
const freeOSMemoryMinPixels = 4_000_000

// copyFile writes to a temporary sibling and renames it into place. io.Copy
// can use optimized file-to-file copies; a bounded reader still protects
// against a local source growing after its initial size check.
func copyFile(dst string, src io.Reader, limit int64) error {
	return writeFileAtomic(dst, func(out *os.File) error {
		n, err := io.Copy(out, io.LimitReader(src, limit+1))
		if err == nil && n > limit {
			err = errMediaTooLarge
		}
		return err
	})
}

// writeFileAtomic writes a temporary sibling of dst, flushes it to stable
// storage and renames it into place. A failed or interrupted write can never
// truncate an existing file at dst.
func writeFileAtomic(dst string, write func(*os.File) error) error {
	out, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*"+filepath.Ext(dst))
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	writeErr := write(out)
	if writeErr == nil {
		writeErr = out.Sync()
	}
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, dst)
}

var mimeToExt = map[string]string{
	"image/jpeg": "jpg", "image/png": "png", "image/gif": "gif",
	"image/webp": "webp", "image/bmp": "bmp", "image/tiff": "tiff",
	"video/mp4": "mp4", "video/webm": "webm",
}

func isVideo(ext string) bool { return config.IsVideoExt(ext) }

func storedExt(ext string, lossless bool) string {
	if !lossless && (ext == "bmp" || ext == "tiff") {
		return "jpg"
	}
	return ext
}

func canUseLosslessMode() bool {
	return config.Current.Compression.Quality == 100 && config.Current.Compression.Scale == 100
}

// Upload modes for the optional "mode" form field.
const (
	// uploadModeReplace swaps the file behind the URL. It is the default and the
	// only behaviour Lanpaper had before playlists existed.
	uploadModeReplace = "replace"
	// uploadModeAppend adds a playlist item behind the same URL, leaving the
	// live file (and therefore every existing embed) untouched.
	uploadModeAppend = "append"
)

func uploadMode(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", uploadModeReplace:
		return uploadModeReplace, true
	case uploadModeAppend:
		return uploadModeAppend, true
	}
	return "", false
}

// formFlag reads an optional boolean form field ("1", "true", "yes", "on").
// Anything else — including an absent field — is false, so existing clients
// keep the behaviour they had.
func formFlag(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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

// thumbnail fits src into maxW x maxH, never upscaling.
func thumbnail(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	return resize(src, min(float64(maxW)/float64(b.Dx()), float64(maxH)/float64(b.Dy())))
}

// scaleImage applies the configured COMPRESSION_SCALE percentage.
func scaleImage(src image.Image, scalePercent int) image.Image {
	return resize(src, float64(scalePercent)/100)
}

// resize downscales src by scale with a bilinear kernel widened to the scale
// factor (so every source pixel contributes). It uses Kernel.Transform, not
// Kernel.Scale: Scale allocates a dstWidth x srcHeight float64 buffer (575 MB
// to halve a 36 MP photo), while Transform needs no temporary memory and
// produces the same pixels to within one level of rounding (resize_test.go).
func resize(src image.Image, scale float64) image.Image {
	if scale >= 1 {
		return src
	}
	b := src.Bounds()
	w := max(1, int(float64(b.Dx())*scale))
	h := max(1, int(float64(b.Dy())*scale))
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	sx := float64(w) / float64(b.Dx())
	sy := float64(h) / float64(b.Dy())
	s2d := f64.Aff3{sx, 0, -float64(b.Min.X) * sx, 0, sy, -float64(b.Min.Y) * sy}
	xdraw.BiLinear.Transform(dst, s2d, src, b, draw.Src, nil)
	return dst
}

// extendDeadline lifts the server-wide read/write timeouts for a long-running
// admin request. It is only reached after authentication; public requests
// keep the short server defaults.
func extendDeadline(w http.ResponseWriter, d time.Duration) {
	rc := http.NewResponseController(w)
	deadline := time.Now().Add(d)
	_ = rc.SetReadDeadline(deadline)
	_ = rc.SetWriteDeadline(deadline)
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
	// Large files over slow links need more than the default read timeout.
	extendDeadline(w, time.Duration(config.UploadBaseTimeout+maxRequest/config.UploadMinBytesPerSec)*time.Second)
	r.Body = http.MaxBytesReader(w, r.Body, maxRequest)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
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
	mode, modeOK := uploadMode(r.FormValue("mode"))
	if !modeOK {
		http.Error(w, "Invalid mode (expected replace or append)", http.StatusBadRequest)
		return
	}
	// autoCreate lets a single request create the link and push its first file,
	// which is what a webhook or a publish key needs. Without the flag an upload
	// to an unknown name is still rejected, so a typo cannot silently create a
	// link and an existing client's behaviour does not change.
	autoCreate := formFlag(r.FormValue("autoCreate"))
	urlStr := r.FormValue("url")
	if len(urlStr) > 2048 {
		http.Error(w, "URL too long", http.StatusBadRequest)
		return
	}
	unlock := storage.LockLinks(name)
	defer unlock()
	prev, exists := storage.Global.Get(name)
	if !exists {
		if !autoCreate {
			http.Error(w, "Link does not exist", http.StatusBadRequest)
			return
		}
		created, err := createLinkForUpload(name, r)
		if err != nil {
			if errors.Is(err, errInvalidLinkDefaults) {
				http.Error(w, "Invalid access level or category", http.StatusBadRequest)
				return
			}
			writeStoreError(w, err)
			return
		}
		prev = created
	}
	// A playlist item lives next to the live file, so the item count and the
	// target directory are settled before any media is downloaded or decoded.
	itemID := 0
	if mode == uploadModeAppend {
		if !prev.HasImage {
			http.Error(w, "Link has no media to add to", http.StatusBadRequest)
			return
		}
		if len(prev.Items) >= config.Current.PlaylistMax {
			http.Error(w, fmt.Sprintf("Playlist is full (max %d items)", config.Current.PlaylistMax), http.StatusConflict)
			return
		}
		itemID = storage.NextItemID(prev.Items)
		if err := os.MkdirAll(storage.ItemsDirPath(name), config.DataDirPerm); err != nil {
			http.Error(w, "Storage unavailable", http.StatusInternalServerError)
			return
		}
	}
	if err := os.MkdirAll(config.MediaDir, config.DataDirPerm); err != nil {
		http.Error(w, "Storage unavailable", http.StatusInternalServerError)
		return
	}
	if err := os.MkdirAll(config.PreviewDir, config.DataDirPerm); err != nil {
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
				writeExternalFileError(w, err)
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
		if isVideo(ext) != isVideo(nameExt) {
			http.Error(w, "Media type does not match local file", http.StatusBadRequest)
			return
		}
	}

	video := isVideo(ext)
	lossless := !video && canUseLosslessMode()
	saveExt := storedExt(ext, lossless)
	// Staging happens in the directory the file will end up in, so publishing is
	// always a rename within one directory (no cross-device surprise).
	stageDir := config.MediaDir
	if mode == uploadModeAppend {
		stageDir = storage.ItemsDirPath(name)
	}
	imageStage, err := stagePath(stageDir, saveExt)
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
		// Playlist items get no thumbnail: the panel shows one preview per link,
		// so encoding a WebP for every item would multiply upload CPU and disk
		// use for metadata nobody looks at.
		if mode == uploadModeReplace {
			previewStage, err = stagePath(config.PreviewDir, "webp")
			if err != nil {
				writeUploadError(w, err)
				return
			}
			defer os.Remove(previewStage)
			if err := savePreview(img, previewStage); err != nil {
				writeUploadError(w, err)
				return
			}
		}
		if lossless {
			if _, err := source.Seek(0, io.SeekStart); err != nil {
				writeUploadError(w, err)
				return
			}
			err = copyFile(imageStage, source, maxBytes)
		} else {
			err = saveImage(img, saveExt, imageStage, config.Current.Compression.Quality)
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
	if mode == uploadModeAppend {
		imagePath = storage.ItemPath(name, itemID, saveExt)
	}
	previewPath := ""
	previewURL := ""
	if !video && mode == uploadModeReplace {
		previewPath = storage.PreviewFilePath(name)
		previewURL = "/api/preview/" + name
	}
	imagePub, err := publishStaged(imageStage, imagePath, maxBytes)
	if err != nil {
		writeUploadError(w, err)
		return
	}

	if mode == uploadModeAppend {
		// A playlist item never touches the live file, its preview or its
		// history: the URL keeps serving exactly what it served before, and the
		// rotation settings decide when the new file joins in.
		item := storage.PlaylistItem{
			ID:        itemID,
			Ext:       saveExt,
			SizeBytes: fi.Size(),
			ModTime:   fi.ModTime().Unix(),
			AddedAt:   time.Now().Unix(),
		}
		appended, err := storage.Global.Update(name, func(wp *storage.Wallpaper) error {
			wp.Items = storage.AppendItem(wp.Items, item)
			return nil
		})
		if err != nil {
			imagePub.rollback()
			writeStoreError(w, err)
			return
		}
		imagePub.finish()
		log.Printf("Appended playlist item #%d to %s (%s, %d KB)", itemID, name, saveExt, fi.Size()/1024)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(toResponse(appended))
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
	// Version history: the backup publishStaged kept IS the file this upload
	// replaced, so archiving it costs one rename — no extra read, no extra write
	// and no extra disk block for as long as the link stays. It runs after the
	// metadata commit and is best effort: a failed archive must never fail an
	// upload that already succeeded.
	if config.Current.History.Limit > 0 && imagePub.backup != "" {
		archived, err := storage.ArchiveReplaced(prev, imagePub.backup, prev.MIMEType, config.Current.History.Limit)
		if err != nil {
			log.Printf("Upload: could not archive the previous version of %s: %v", name, err)
		} else if archived != nil {
			updated = archived
		}
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
	if fingerprint := middleware.PublisherFingerprint(r); fingerprint != "" {
		log.Printf("Uploaded: %s (%s, %d KB) with publish key %s", name, saveExt, fi.Size()/1024, fingerprint)
	} else {
		log.Printf("Uploaded: %s (%s, %d KB)", name, saveExt, fi.Size()/1024)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(toResponse(updated))
}

// errInvalidLinkDefaults rejects an auto-created link whose requested access
// level or category is not on the allow-list.
var errInvalidLinkDefaults = errors.New("invalid access level or category")

// createLinkForUpload creates the link an upload targets when the client set
// autoCreate, so a webhook or a publish key can push media in one request
// instead of create-then-upload. The caller holds the link lock.
func createLinkForUpload(name string, r *http.Request) (*storage.Wallpaper, error) {
	rawLevel := strings.TrimSpace(r.FormValue("accessLevel"))
	if rawLevel != "" && !isValidAccessLevel(rawLevel) {
		return nil, errInvalidLinkDefaults
	}
	level := storage.NormalizeAccessLevel(rawLevel)
	category := strings.TrimSpace(r.FormValue("category"))
	if category != "" && !isValidCategory(category) {
		return nil, errInvalidLinkDefaults
	}
	if category == "" {
		category = "other"
	}
	wp := &storage.Wallpaper{
		ID: name, LinkName: name, Category: category, ImageURL: "/" + name,
		CreatedAt: time.Now().Unix(), AccessLevel: level,
	}
	if level == config.AccessToken {
		wp.AccessToken = generateAccessToken()
	}
	if err := storage.Global.Create(wp); err != nil {
		return nil, err
	}
	created, exists := storage.Global.Get(name)
	if !exists {
		return nil, storage.ErrNotFound
	}
	return created, nil
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
func saveImage(img image.Image, format, path string, quality int) error {
	return writeFileAtomic(path, func(out *os.File) error {
		return encodeImage(out, img, format, quality)
	})
}

// savePreview writes the WebP thumbnail shown in the admin panel.
func savePreview(img image.Image, path string) error {
	return saveImage(thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", path, config.ThumbnailQuality)
}

func encodeImage(w io.Writer, img image.Image, format string, quality int) error {
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
