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
	uploadLinkMu.m = make(map[string]*sync.Mutex)
}

// ---------------------------------------------------------------------------
// Upload concurrency guards
// ---------------------------------------------------------------------------

var uploadSem chan struct{}

func InitUploadSemaphore(n int) {
	if n <= 0 {
		n = 2
	}
	uploadSem = make(chan struct{}, n)
}

// uploadLinkMu serialises concurrent uploads to the same link name,
// preventing races on file deletion and store updates.
var uploadLinkMu struct {
	sync.Mutex
	m map[string]*sync.Mutex
}

func lockLink(name string) func() {
	uploadLinkMu.Lock()
	mu, ok := uploadLinkMu.m[name]
	if !ok {
		mu = &sync.Mutex{}
		uploadLinkMu.m[name] = mu
	}
	uploadLinkMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// ---------------------------------------------------------------------------
// HTTP transport (cached, rebuilt on config change)
// ---------------------------------------------------------------------------

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

	dialer := &ssrfSafeDialer{inner: &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}}
	t := &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: insecure},
		DialContext:           dialer.DialContext,
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

// ---------------------------------------------------------------------------
// SSRF protection
// ---------------------------------------------------------------------------

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
	if config.Current.AllowPrivateURLFetch {
		return d.inner.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}
	for _, ipAddr := range ips {
		if !utils.IsPrivateOrLocalIP(ipAddr.IP) {
			return d.inner.DialContext(ctx, network, net.JoinHostPort(ipAddr.IP.String(), port))
		}
	}
	log.Printf("[SECURITY] Blocked SSRF attempt: %s resolves only to private IPs", host)
	return nil, errors.New("address is not allowed")
}

// ssrfCheckRedirect blocks redirects that resolve to private/loopback IPs.
func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
	maxRedirects := 5
	if config.Current.AllowPrivateURLFetch {
		maxRedirects = 10
	}
	if len(via) >= maxRedirects {
		return errors.New("too many redirects")
	}
	if config.Current.AllowPrivateURLFetch {
		return nil
	}
	host := req.URL.Hostname()
	ips, err := net.DefaultResolver.LookupIPAddr(req.Context(), host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("redirect DNS failed for %s", host)
	}
	for _, ipAddr := range ips {
		if utils.IsPrivateOrLocalIP(ipAddr.IP) {
			log.Printf("[SECURITY] Blocked redirect SSRF to private IP via %s", host)
			return errors.New("redirect to private address is not allowed")
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Atomic file I/O
// ---------------------------------------------------------------------------

// atomicWriteFile writes r to dst via a temp file + rename so readers never
// observe a partially-written file.
func atomicWriteFile(dst string, r io.Reader) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".upload-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	bw := bufio.NewWriterSize(tmp, config.FileCopyBufferSize)
	if _, err := io.Copy(bw, r); err != nil {
		tmp.Close(); os.Remove(tmpName)
		return fmt.Errorf("write: %w", err)
	}
	if err := bw.Flush(); err != nil {
		tmp.Close(); os.Remove(tmpName)
		return fmt.Errorf("flush: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close(); os.Remove(tmpName)
		return fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// atomicSaveImage encodes img to a temp file then renames it into place.
func atomicSaveImage(img image.Image, format, dst string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".img-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if err := encodeImage(tmp, img, format); err != nil {
		tmp.Close(); os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close(); os.Remove(tmpName)
		return fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// copyFile atomically copies r (or srcPath if r is nil) to dst.
func copyFile(srcPath, dst string, r io.Reader) error {
	if r == nil {
		f, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}
		defer f.Close()
		r = f
	}
	return atomicWriteFile(dst, r)
}

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

func imagePath(linkName, ext string) string {
	return filepath.Join("static", "images", linkName+"."+ext)
}

func previewFilePath(linkName string) string {
	return filepath.Join("static", "images", "previews", linkName+".webp")
}

func previewURLPath(linkName string) string {
	return "/static/images/previews/" + linkName + ".webp"
}

// ---------------------------------------------------------------------------
// MIME / format helpers
// ---------------------------------------------------------------------------

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

// storedExt returns the extension to use when saving.
// In lossless mode the original format is preserved.
// In compression mode BMP/TIFF are converted to JPEG.
func storedExt(ext string, lossless bool) string {
	if lossless {
		return ext
	}
	if ext == "bmp" || ext == "tiff" {
		return "jpg"
	}
	return ext
}

// canUseLosslessMode reports whether the config allows a byte-for-byte copy
// (quality=100, scale=100).
func canUseLosslessMode() bool {
	return config.Current.Compression.Quality == 100 && config.Current.Compression.Scale == 100
}

func isVideo(ext string) bool { return ext == "mp4" || ext == "webm" }

// ---------------------------------------------------------------------------
// Image processing helpers
// ---------------------------------------------------------------------------

// checkImageDimensions returns an error if the image exceeds MaxImageDimension.
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
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func scaleImage(src image.Image, scalePercent int) image.Image {
	if scalePercent >= 100 {
		return src
	}
	b := src.Bounds()
	scale := float64(scalePercent) / 100.0
	newW := max(1, int(float64(b.Dx())*scale))
	newH := max(1, int(float64(b.Dy())*scale))
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
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

// ---------------------------------------------------------------------------
// Image loaders
// ---------------------------------------------------------------------------

func loadLocalImage(ctx context.Context, path string) (image.Image, string, []byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, "", nil, errors.New("file not found")
	}
	defer f.Close()

	// Dimension check first (requires seek back afterwards).
	if err := checkImageDimensions(f); err != nil {
		log.Printf("[SECURITY] Rejected local image %s: %v", path, err)
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

	ext, ok := mimeToExt[http.DetectContentType(fileData)]
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

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,video/*;q=0.9")

	resp, err := (&http.Client{
		Transport:     getTransport(),
		CheckRedirect: ssrfCheckRedirect,
	}).Do(req)
	if err != nil {
		log.Printf("downloadImage: network error fetching %s: %v", urlStr, err)
		return nil, "", nil, errors.New("network error")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Reject clearly non-media Content-Type before reading the body.
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		base := strings.ToLower(strings.SplitN(ct, ";", 2)[0])
		if !strings.HasPrefix(base, "image/") && !strings.HasPrefix(base, "video/") &&
			base != "application/octet-stream" {
			log.Printf("[SECURITY] Rejected download %s — Content-Type %q is not media", urlStr, ct)
			return nil, "", nil, errors.New("unsupported content type")
		}
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	if resp.ContentLength > maxBytes {
		log.Printf("[SECURITY] Rejected download Content-Length %d (max %d)", resp.ContentLength, maxBytes)
		return nil, "", nil, errors.New("file too large")
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", nil, errors.New("read error")
	}
	if int64(len(buf)) > maxBytes {
		log.Printf("[SECURITY] Rejected download body > %d bytes", maxBytes)
		return nil, "", nil, errors.New("file too large")
	}

	if err := checkImageDimensions(bytes.NewReader(buf)); err != nil {
		log.Printf("[SECURITY] Rejected remote image %s: %v", urlStr, err)
		return nil, "", nil, errors.New("image dimensions too large")
	}

	ext, ok := mimeToExt[http.DetectContentType(buf)]
	if !ok {
		return nil, "", nil, errors.New("unsupported format")
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

// ---------------------------------------------------------------------------
// Upload handler
// ---------------------------------------------------------------------------

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
		log.Printf("[SECURITY] Rejected upload with Content-Length %d (max %d)", r.ContentLength, maxBytes)
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

	unlockLink := lockLink(linkName)
	defer unlockLink()

	oldWp, exists := storage.Global.Get(linkName)
	if !exists {
		http.Error(w, "Link does not exist", http.StatusBadRequest)
		return
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
		} else {
			if !utils.IsValidLocalPath(urlStr) {
				log.Printf("[SECURITY] Blocked invalid path: %s", urlStr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}
			absPath, _, pathErr := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), urlStr)
			if pathErr != nil {
				log.Printf("[SECURITY] Path validation failed for %s: %v", urlStr, pathErr)
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
			log.Printf("[SECURITY] Rejected file %s size %d (max %d)", header.Filename, header.Size, maxBytes)
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
			log.Printf("[SECURITY] Rejected %s — unsupported MIME type", safeFilename)
			http.Error(w, "Unsupported file type", http.StatusBadRequest)
			return
		}
		ext = e
		video = isVideo(ext)

		if err := utils.ValidateFileType(head, ext); err != nil {
			log.Printf("[SECURITY] Magic bytes failed for %s: %v", safeFilename, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}

		if !video {
			if dimErr := checkImageDimensions(upFile); dimErr != nil {
				log.Printf("[SECURITY] Rejected image %s: %v", safeFilename, dimErr)
				http.Error(w, "Image dimensions too large", http.StatusBadRequest)
				return
			}
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error after dimension check: %v", err)
				http.Error(w, "File seek error", http.StatusInternalServerError)
				return
			}
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

	// For downloaded/local files: validate magic bytes and set lossless flag.
	if len(fileData) > 0 && !video && !losslessMode {
		if err := utils.ValidateFileType(fileData, ext); err != nil {
			log.Printf("[SECURITY] Magic bytes failed for link %s: %v", linkName, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}
		if canUseLosslessMode() {
			losslessMode = true
			log.Printf("Lossless mode: downloaded %s", linkName)
		}
	}

	if oldWp != nil && oldWp.HasImage {
		removeFiles(oldWp.ImagePath, oldWp.PreviewPath)
	}

	saveExt := storedExt(ext, losslessMode)
	origPath := imagePath(linkName, saveExt)
	prevPath := previewFilePath(linkName)

	switch {
	case video:
		var copyErr error
		switch {
		case urlStr == "":
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error before video copy: %v", err)
				http.Error(w, "Failed to prepare video file", http.StatusInternalServerError)
				return
			}
			copyErr = copyFile("", origPath, upFile)
		case !strings.HasPrefix(urlStr, "http"):
			absPath, _, pathErr := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), urlStr)
			if pathErr != nil {
				log.Printf("[SECURITY] Path validation failed for video %s: %v", urlStr, pathErr)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}
			copyErr = copyFile(absPath, origPath, nil)
		case len(fileData) > 0:
			copyErr = copyFile("", origPath, bytes.NewReader(fileData))
		}
		if copyErr != nil {
			log.Printf("Error saving video %s: %v", origPath, copyErr)
			http.Error(w, "Failed to save video", http.StatusInternalServerError)
			return
		}
		prevPath = ""

	case losslessMode:
		var copyErr error
		switch {
		case len(fileData) > 0:
			copyErr = copyFile("", origPath, bytes.NewReader(fileData))
		case upFile != nil:
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error before lossless copy: %v", err)
				http.Error(w, "Failed to prepare file", http.StatusInternalServerError)
				return
			}
			copyErr = copyFile("", origPath, upFile)
		}
		if copyErr != nil {
			log.Printf("Error saving lossless image %s: %v", origPath, copyErr)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}
		// Decode for thumbnail from already-buffered bytes.
		var previewSrc image.Image
		if len(fileData) > 0 {
			previewSrc, _, err = image.Decode(bytes.NewReader(fileData))
		} else if upFile != nil {
			if _, seekErr := upFile.Seek(0, io.SeekStart); seekErr == nil {
				previewSrc, _, err = image.Decode(upFile)
			}
		}
		if err != nil || previewSrc == nil {
			log.Printf("Warning: failed to generate preview for %s: %v", linkName, err)
			prevPath = ""
		} else if saveErr := atomicSaveImage(thumbnail(previewSrc, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", prevPath); saveErr != nil {
			log.Printf("Error saving preview %s: %v", prevPath, saveErr)
			prevPath = ""
		}

	default:
		img = scaleImage(img, config.Current.Compression.Scale)
		if err := atomicSaveImage(img, saveExt, origPath); err != nil {
			log.Printf("Error saving image %s: %v", origPath, err)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}
		if err := atomicSaveImage(thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", prevPath); err != nil {
			log.Printf("Error saving preview %s: %v", prevPath, err)
			removeFiles(origPath, prevPath)
			http.Error(w, "Preview generation failed", http.StatusInternalServerError)
			return
		}
	}

	fi, err := os.Stat(origPath)
	if err != nil {
		log.Printf("Error stating %s: %v", origPath, err)
		http.Error(w, "Failed to stat file", http.StatusInternalServerError)
		return
	}

	createdAt := time.Now().Unix()
	if oldWp != nil {
		createdAt = oldWp.CreatedAt
	}

	// Carry over user-set fields so a re-upload never silently drops them.
	var isPinned bool
	var pinnedAt int64
	var category string
	if oldWp != nil {
		isPinned, pinnedAt, category = oldWp.IsPinned, oldWp.PinnedAt, oldWp.Category
	}

	previewURL := ""
	if prevPath != "" {
		previewURL = previewURLPath(linkName)
	}

	wp := &storage.Wallpaper{
		ID:          linkName,
		LinkName:    linkName,
		Category:    category,
		ImageURL:    "/static/images/" + linkName + "." + saveExt,
		Preview:     previewURL,
		HasImage:    true,
		MIMEType:    saveExt,
		SizeBytes:   fi.Size(),
		ModTime:     fi.ModTime().Unix(),
		CreatedAt:   createdAt,
		IsPinned:    isPinned,
		PinnedAt:    pinnedAt,
		ImagePath:   origPath,
		PreviewPath: prevPath,
	}
	storage.Global.Set(linkName, wp)
	if err := storage.Global.Save(); err != nil {
		log.Printf("Error saving after upload: %v — rolling back", err)
		storage.Global.Delete(linkName)
		removeFiles(origPath, prevPath)
		http.Error(w, "Failed to persist upload", http.StatusInternalServerError)
		return
	}
	if config.Current.MaxImages > 0 {
		go storage.PruneOldImages(config.Current.MaxImages)
	}

	mode := "compressed"
	switch {
	case losslessMode:
		mode = "lossless"
	case video:
		mode = "video"
	}
	log.Printf("Uploaded: %s (%s, %d KB, %s)", linkName, saveExt, fi.Size()/1024, mode)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(wp); err != nil {
		log.Printf("Error encoding upload response: %v", err)
	}
}
