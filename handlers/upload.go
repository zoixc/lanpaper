package handlers

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
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
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

func init() {
	image.RegisterFormat("webp", "RIFF????WEBP", webp.Decode, webp.DecodeConfig)
}

var uploadSem chan struct{}

func InitUploadSemaphore(n int) {
	if n <= 0 {
		n = 2
	}
	uploadSem = make(chan struct{}, n)
}

var (
	transportMu     sync.Mutex
	cachedTransport *http.Transport
	cachedProxyHost string
	cachedInsecure  bool
)

func getTransport() *http.Transport {
	transportMu.Lock()
	defer transportMu.Unlock()

	proxyHost := config.Current.ProxyHost
	insecure := config.Current.InsecureSkipVerify
	if cachedTransport != nil && cachedProxyHost == proxyHost && cachedInsecure == insecure {
		return cachedTransport
	}
	if cachedTransport != nil {
		cachedTransport.CloseIdleConnections()
	}

	t := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		DialContext: (&ssrfSafeDialer{inner: &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	if proxyHost != "" {
		proxyURL := &url.URL{
			Scheme: config.Current.ProxyType,
			Host:   net.JoinHostPort(proxyHost, config.Current.ProxyPort),
		}
		if config.Current.ProxyUsername != "" {
			proxyURL.User = url.UserPassword(config.Current.ProxyUsername, config.Current.ProxyPassword)
		}
		t.Proxy = http.ProxyURL(proxyURL)
	}
	cachedTransport, cachedProxyHost, cachedInsecure = t, proxyHost, insecure
	return t
}

type ssrfSafeDialer struct{ inner *net.Dialer }

func (d *ssrfSafeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("DNS resolution failed for %s", host)
	}
	var safeIP string
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			continue
		}
		isPrivate := false
		for _, cidr := range utils.PrivateRanges() {
			if cidr.Contains(ip) {
				isPrivate = true
				break
			}
		}
		if !isPrivate {
			safeIP = ip.String()
			break
		}
	}
	if safeIP == "" {
		return nil, errors.New("address is not allowed")
	}
	return d.inner.DialContext(ctx, network, net.JoinHostPort(safeIP, port))
}

// copyFile copies srcPath (or the reader r, when set) into a temporary file
// next to dst and renames it into place, so a crash or error mid-copy never
// leaves a truncated file at dst.
func copyFile(srcPath, dst string, r io.Reader) error {
	if r == nil {
		f, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}
		defer f.Close()
		r = f
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*"+filepath.Ext(dst))
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	bw := bufio.NewWriterSize(tmp, config.FileCopyBufferSize)
	_, copyErr := io.Copy(bw, r)
	flushErr := bw.Flush()
	closeErr := tmp.Close()
	if copyErr != nil {
		copyErr = fmt.Errorf("copy: %w", copyErr)
	}
	if copyErr != nil || flushErr != nil || closeErr != nil {
		os.Remove(tmpName)
		if copyErr != nil {
			return copyErr
		}
		if flushErr != nil {
			return fmt.Errorf("flush: %w", flushErr)
		}
		return fmt.Errorf("close: %w", closeErr)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

var mimeToExt = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/gif":  "gif",
	"image/webp": "webp",
	"image/bmp":  "bmp",
	"image/tiff": "tiff",
	"video/mp4":  "mp4",
	"video/webm": "webm",
}

func normalizeFormat(format string) string {
	if format == "jpeg" {
		return "jpg"
	}
	return format
}

// storedExt returns the file extension to use for storage.
// In lossless mode, the original format is preserved.
// In compression mode, BMP/TIFF are converted to JPEG.
func storedExt(ext string, lossless bool) string {
	if lossless {
		return ext
	}
	if ext == "bmp" || ext == "tiff" {
		return "jpg"
	}
	return ext
}

// canUseLosslessMode returns true if files can be copied byte-for-byte
// without re-encoding (quality=100, scale=100, any supported image format).
func canUseLosslessMode() bool {
	return config.Current.Compression.Quality == 100 && config.Current.Compression.Scale == 100
}

// checkImageDimensions returns an error if the image exceeds the allowed
// dimensions. Unlike before, a decode error is now propagated so callers
// can decide whether to reject the file.
func checkImageDimensions(r io.ReadSeeker) error {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return fmt.Errorf("could not read image config: %w", err)
	}
	if cfg.Width > config.MaxImageDimension || cfg.Height > config.MaxImageDimension {
		return fmt.Errorf("image %dx%d exceeds %dx%d limit",
			cfg.Width, cfg.Height, config.MaxImageDimension, config.MaxImageDimension)
	}
	return nil
}

func thumbnail(src image.Image, maxW, maxH int) image.Image {
	b := src.Bounds()
	scale := min(float64(maxW)/float64(b.Dx()), float64(maxH)/float64(b.Dy()))
	if scale >= 1 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)))
	// draw.Src: the destination is freshly allocated (fully transparent),
	// so Src produces exactly the same pixels as Over but skips the
	// per-pixel alpha-blending work.
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func scaleImage(src image.Image, scalePercent int) image.Image {
	if scalePercent >= 100 {
		return src
	}
	b := src.Bounds()
	scale := float64(scalePercent) / 100.0
	newW := int(float64(b.Dx()) * scale)
	newH := int(float64(b.Dy()) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

func isVideo(ext string) bool { return ext == "mp4" || ext == "webm" }

// linkUploadLocks serializes concurrent uploads targeting the same link so
// two uploads cannot interleave file writes/removals for one link.
var linkUploadLocks sync.Map

func lockLink(linkName string) func() {
	mu, _ := linkUploadLocks.LoadOrStore(linkName, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

func Upload(w http.ResponseWriter, r *http.Request) {
	select {
	case uploadSem <- struct{}{}:
		defer func() { <-uploadSem }()
	default:
		http.Error(w, "Too many concurrent uploads", http.StatusTooManyRequests)
		return
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	if r.ContentLength > maxBytes {
		log.Printf("Security: rejected upload with Content-Length %d (max %d)", r.ContentLength, maxBytes)
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	if err := r.ParseMultipartForm(maxBytes); err != nil {
		http.Error(w, "File too large", http.StatusBadRequest)
		return
	}

	linkName := r.FormValue("linkName")
	if !isValidLinkName(linkName) {
		http.Error(w, "Invalid link name", http.StatusBadRequest)
		return
	}

	// Serialize concurrent uploads for the same link, then read the current
	// state so the rollback/cleanup below always works with fresh data.
	unlock := lockLink(linkName)
	defer unlock()

	oldWp, exists := storage.Global.Get(linkName)
	if !exists {
		http.Error(w, "Link does not exist", http.StatusBadRequest)
		return
	}
	// Keep a copy of the previous state: it is restored if the upload fails
	// halfway, and tells us which old files to clean up on success.
	var prev *storage.Wallpaper
	if oldWp != nil {
		clone := *oldWp
		prev = &clone
	}

	var (
		img          image.Image
		ext          string
		err          error
		video        bool
		fileData     []byte
		upFile       multipart.File
		losslessMode bool
	)

	urlStr := r.FormValue("url")
	if urlStr != "" {
		if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
			img, ext, fileData, err = downloadImage(r.Context(), urlStr)
			if err == nil && isVideo(ext) {
				video = true
			}
		} else {
			if !utils.IsValidLocalPath(urlStr) {
				log.Printf("Security: blocked invalid path: %s", urlStr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}
			absPath, _, pathErr := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), urlStr)
			if pathErr != nil {
				log.Printf("Security: path validation failed for %s: %v", urlStr, pathErr)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}
			ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(absPath)), ".")
			if isVideo(ext) {
				video = true
			} else {
				img, ext, fileData, err = loadLocalImage(r.Context(), absPath)
			}
		}
		if err != nil {
			log.Printf("Image load error for %s: %v", linkName, err)
			http.Error(w, "Failed to load image", http.StatusBadRequest)
			return
		}
	} else {
		var header *multipart.FileHeader
		upFile, header, err = r.FormFile("file")
		if err != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer upFile.Close()

		if header.Size > maxBytes {
			log.Printf("Security: rejected file %s size %d (max %d)", header.Filename, header.Size, maxBytes)
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
			return
		}
		safeFilename := utils.SanitizeFilename(header.Filename)

		head := make([]byte, 512)
		n, readErr := upFile.Read(head)
		if readErr != nil && readErr != io.EOF {
			http.Error(w, "Read error", http.StatusBadRequest)
			return
		}
		head = head[:n]
		if _, err := upFile.Seek(0, io.SeekStart); err != nil {
			log.Printf("Error seeking file: %v", err)
			http.Error(w, "File seek error", http.StatusInternalServerError)
			return
		}

		e, ok := mimeToExt[http.DetectContentType(head)]
		if !ok {
			log.Printf("Security: rejected %s — unsupported MIME type", safeFilename)
			http.Error(w, "Unsupported file type", http.StatusBadRequest)
			return
		}
		ext = e
		video = isVideo(ext)

		if err := utils.ValidateFileType(head, ext); err != nil {
			log.Printf("Security: magic bytes failed for %s: %v", safeFilename, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}

		if !video {
			if dimErr := checkImageDimensions(upFile); dimErr != nil {
				log.Printf("Security: rejected image %s: %v", safeFilename, dimErr)
				http.Error(w, "Image dimensions too large", http.StatusBadRequest)
				return
			}
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error after dimension check: %v", err)
				http.Error(w, "File seek error", http.StatusInternalServerError)
				return
			}

			// Check lossless mode BEFORE decoding
			if canUseLosslessMode() {
				losslessMode = true
				log.Printf("Lossless mode: %s (quality=%d, scale=%d) — skipping decode",
					safeFilename, config.Current.Compression.Quality, config.Current.Compression.Scale)
				fileData, err = io.ReadAll(upFile)
				if err != nil {
					log.Printf("Error reading file data: %v", err)
					http.Error(w, "Read error", http.StatusInternalServerError)
					return
				}
			} else {
				log.Printf("Compression mode: %s (quality=%d, scale=%d)",
					safeFilename, config.Current.Compression.Quality, config.Current.Compression.Scale)
				if img, _, err = image.Decode(upFile); err != nil {
					log.Printf("Image decode error for %s: %v", safeFilename, err)
					http.Error(w, "Invalid image", http.StatusBadRequest)
					return
				}
			}
		}
	}

	if len(fileData) > 0 && !video && !losslessMode {
		if err := utils.ValidateFileType(fileData, ext); err != nil {
			log.Printf("Security: magic bytes failed for link %s: %v", linkName, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}
		// Check lossless for downloaded/local files
		if canUseLosslessMode() {
			losslessMode = true
			log.Printf("Lossless mode: downloaded %s", linkName)
		}
	}

	// NOTE: the previous image files are deliberately NOT removed yet.
	// They are only cleaned up after the new content is fully written and
	// persisted, so a failed save (disk full, permissions, ...) can no longer
	// destroy the last working image of a link.

	saveExt := storedExt(ext, losslessMode)
	originalPath := filepath.Join("static", "images", linkName+"."+saveExt)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")
	if video {
		previewPath = ""
	}

	if video {
		var copyErr error
		if urlStr == "" {
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error before video copy: %v", err)
				http.Error(w, "Failed to prepare video file", http.StatusInternalServerError)
				return
			}
			copyErr = copyFile("", originalPath, upFile)
		} else if !strings.HasPrefix(urlStr, "http") {
			absPath, _, pathErr := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), urlStr)
			if pathErr != nil {
				log.Printf("Security: path validation failed for video %s: %v", urlStr, pathErr)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}
			copyErr = copyFile(absPath, originalPath, nil)
		} else if len(fileData) > 0 {
			copyErr = copyFile("", originalPath, bytes.NewReader(fileData))
		}
		if copyErr != nil {
			log.Printf("Error saving video %s: %v", originalPath, copyErr)
			http.Error(w, "Failed to save video", http.StatusInternalServerError)
			return
		}
	} else if losslessMode {
		// Lossless mode: copy file directly without re-encoding
		var copyErr error
		if len(fileData) > 0 {
			copyErr = copyFile("", originalPath, bytes.NewReader(fileData))
		} else if urlStr == "" && upFile != nil {
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error before lossless copy: %v", err)
				http.Error(w, "Failed to prepare file", http.StatusInternalServerError)
				return
			}
			copyErr = copyFile("", originalPath, upFile)
		}
		if copyErr != nil {
			log.Printf("Error saving lossless image %s: %v", originalPath, copyErr)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}
		// Generate preview by decoding from the already-read bytes
		var previewImg image.Image
		if len(fileData) > 0 {
			previewImg, _, err = image.Decode(bytes.NewReader(fileData))
		} else if upFile != nil {
			if _, seekErr := upFile.Seek(0, io.SeekStart); seekErr == nil {
				previewImg, _, err = image.Decode(upFile)
			}
		}
		if err != nil || previewImg == nil {
			log.Printf("Warning: failed to generate preview for %s: %v", linkName, err)
			previewPath = ""
		} else {
			if err := saveImage(thumbnail(previewImg, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", previewPath); err != nil {
				log.Printf("Error saving preview %s: %v", previewPath, err)
				previewPath = ""
			}
		}
	} else {
		// Normal mode: decode, process, and re-encode.
		// The preview is written first: if it fails, the previous image of
		// this link is still completely intact.
		img = scaleImage(img, config.Current.Compression.Scale)

		if err := saveImage(thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", previewPath); err != nil {
			log.Printf("Error saving preview %s: %v", previewPath, err)
			http.Error(w, "Preview generation failed", http.StatusInternalServerError)
			return
		}
		if err := saveImage(img, saveExt, originalPath); err != nil {
			log.Printf("Error saving image %s: %v", originalPath, err)
			// Drop the freshly written preview; the old image (if any) is
			// untouched and can be re-previewed via regenerate-previews.
			removeFiles("", previewPath)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}
	}

	fi, err := os.Stat(originalPath)
	if err != nil {
		log.Printf("Error stating %s: %v", originalPath, err)
		http.Error(w, "Failed to stat file", http.StatusInternalServerError)
		return
	}

	createdAt := time.Now().Unix()
	if oldWp != nil {
		createdAt = oldWp.CreatedAt
	}
	previewURL := ""
	if previewPath != "" {
		previewURL = "/static/images/previews/" + linkName + ".webp"
	}

	wp := &storage.Wallpaper{
		ID:          linkName,
		LinkName:    linkName,
		ImageURL:    "/static/images/" + linkName + "." + saveExt,
		Preview:     previewURL,
		HasImage:    true,
		MIMEType:    saveExt,
		SizeBytes:   fi.Size(),
		ModTime:     fi.ModTime().Unix(),
		CreatedAt:   createdAt,
		ImagePath:   originalPath,
		PreviewPath: previewPath,
	}
	storage.Global.Set(linkName, wp)
	if err := storage.Global.Save(); err != nil {
		log.Printf("Error saving after upload: %v — rolling back", err)
		// Restore the previous state of the link instead of deleting it.
		if prev != nil {
			storage.Global.Set(linkName, prev)
		} else {
			storage.Global.Delete(linkName)
		}
		// Remove the newly written files, unless they replaced the previous
		// ones in place (same path) — those are the only copy left.
		if prev == nil || !prev.HasImage || prev.ImagePath != originalPath {
			if err := os.Remove(originalPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing rolled-back image %s: %v", originalPath, err)
			}
		}
		if previewPath != "" && (prev == nil || prev.PreviewPath != previewPath) {
			if err := os.Remove(previewPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing rolled-back preview %s: %v", previewPath, err)
			}
		}
		http.Error(w, "Failed to persist upload", http.StatusInternalServerError)
		return
	}

	// New content is fully persisted — now remove files left over from the
	// previous upload (e.g. the old image with a different extension).
	if prev != nil && prev.HasImage {
		if prev.ImagePath != "" && prev.ImagePath != originalPath {
			if err := os.Remove(prev.ImagePath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing old image %s: %v", prev.ImagePath, err)
			}
		}
		if prev.PreviewPath != "" && prev.PreviewPath != previewPath {
			if err := os.Remove(prev.PreviewPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing old preview %s: %v", prev.PreviewPath, err)
			}
		}
	}

	if config.Current.MaxImages > 0 {
		go storage.PruneOldImages(config.Current.MaxImages)
	}

	mode := "compressed"
	if losslessMode {
		mode = "lossless"
	} else if video {
		mode = "video"
	}
	log.Printf("Uploaded: %s (%s, %d KB, %s)", linkName, saveExt, fi.Size()/1024, mode)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(wp); err != nil {
		log.Printf("Error encoding upload response: %v", err)
	}
}

// saveImage encodes img into a temporary file next to path and renames it
// into place, so readers never observe a half-written file and a failed
// encode never destroys the previous content at path.
func saveImage(img image.Image, format, path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*"+filepath.Ext(path))
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	// os.CreateTemp uses 0600; keep the previous 0644 behaviour so files on
	// mounted volumes stay readable by other tools.
	_ = os.Chmod(tmpName, 0o644)
	encodeErr := encodeImage(tmp, img, format)
	closeErr := tmp.Close()
	if encodeErr == nil {
		encodeErr = closeErr
	}
	if encodeErr != nil {
		os.Remove(tmpName)
		return encodeErr
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
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
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	}
}

func loadLocalImage(ctx context.Context, path string) (image.Image, string, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", nil, errors.New("file not found")
	}
	defer f.Close()

	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, fmt.Errorf("seek: %w", err)
	}
	if dimErr := checkImageDimensions(f); dimErr != nil {
		log.Printf("Security: rejected local image %s: %v", path, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, fmt.Errorf("seek: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", nil, err
	}

	fileData, err := io.ReadAll(f)
	if err != nil {
		return nil, "", nil, fmt.Errorf("read: %w", err)
	}

	mimeType := http.DetectContentType(fileData)
	ext, ok := mimeToExt[mimeType]
	if !ok {
		ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}

	if canUseLosslessMode() {
		log.Printf("Lossless mode: local file %s", path)
		return nil, ext, fileData, nil
	}

	img, format, err := image.Decode(bytes.NewReader(fileData))
	if err != nil {
		log.Printf("Image decode error for %s: %v", path, err)
		return nil, "", nil, errors.New("invalid or unsupported image format")
	}
	return img, normalizeFormat(format), fileData, nil
}

func downloadImage(ctx context.Context, urlStr string) (image.Image, string, []byte, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", nil, errors.New("invalid URL")
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(config.DownloadTimeout)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := (&http.Client{Transport: getTransport()}).Do(req)
	if err != nil {
		return nil, "", nil, errors.New("network error")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	if resp.ContentLength > maxBytes {
		log.Printf("Security: rejected download Content-Length %d (max %d)", resp.ContentLength, maxBytes)
		return nil, "", nil, errors.New("file too large")
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", nil, errors.New("read error")
	}
	if int64(len(buf)) > maxBytes {
		log.Printf("Security: rejected download body > %d bytes", maxBytes)
		return nil, "", nil, errors.New("file too large")
	}

	// Detect the content type first: videos must skip the image dimension
	// check and decoding entirely (image.DecodeConfig cannot parse video
	// containers and would reject every MP4/WebM download).
	mimeType := http.DetectContentType(buf)
	ext, ok := mimeToExt[mimeType]
	if !ok {
		return nil, "", nil, errors.New("unsupported format")
	}

	if isVideo(ext) {
		return nil, ext, buf, nil
	}

	if dimErr := checkImageDimensions(bytes.NewReader(buf)); dimErr != nil {
		log.Printf("Security: rejected remote image %s: %v", urlStr, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}

	if canUseLosslessMode() {
		log.Printf("Lossless mode: downloaded %s", urlStr)
		return nil, ext, buf, nil
	}

	img, format, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", nil, errors.New("invalid or unsupported image format")
	}
	return img, normalizeFormat(format), buf, nil
}
