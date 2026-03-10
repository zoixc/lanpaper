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

	// If allowPrivateURLFetch is enabled, connect to first resolved IP directly
	if config.Current.AllowPrivateURLFetch {
		return d.inner.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
	}

	var safeIP string
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if !utils.IsPrivateOrLocalIP(ip) {
			safeIP = ip.String()
			break
		}
	}
	if safeIP == "" {
		log.Printf("[SECURITY] Blocked SSRF attempt: %s resolves only to private IPs", host)
		return nil, errors.New("address is not allowed")
	}
	return d.inner.DialContext(ctx, network, net.JoinHostPort(safeIP, port))
}

func copyFile(srcPath, dst string, r io.Reader) error {
	if r == nil {
		f, err := os.Open(srcPath)
		if err != nil {
			return fmt.Errorf("open: %w", err)
		}
		defer f.Close()
		r = f
	}
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer out.Close()
	bw := bufio.NewWriterSize(out, config.FileCopyBufferSize)
	if _, err := io.Copy(bw, r); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return bw.Flush()
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

// canUseLosslessMode returns true if the file can be copied byte-for-byte
// without re-encoding (quality=100, scale=100).
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
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
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
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, b, draw.Over, nil)
	return dst
}

func isVideo(ext string) bool { return ext == "mp4" || ext == "webm" }

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
			log.Printf("[SECURITY] Magic bytes failed for link %s: %v", linkName, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}
		// Check lossless for downloaded/local files
		if canUseLosslessMode() {
			losslessMode = true
			log.Printf("Lossless mode: downloaded %s", linkName)
		}
	}

	if oldWp != nil && oldWp.HasImage {
		removeFiles(oldWp.ImagePath, oldWp.PreviewPath)
	}

	saveExt := storedExt(ext, losslessMode)
	originalPath := filepath.Join("static", "images", linkName+"."+saveExt)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")

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
				log.Printf("[SECURITY] Path validation failed for video %s: %v", urlStr, pathErr)
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
		previewPath = ""
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
		// Normal mode: decode, process, and re-encode
		img = scaleImage(img, config.Current.Compression.Scale)

		if err := saveImage(img, saveExt, originalPath); err != nil {
			log.Printf("Error saving image %s: %v", originalPath, err)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}
		if err := saveImage(thumbnail(img, config.ThumbnailMaxWidth, config.ThumbnailMaxHeight), "webp", previewPath); err != nil {
			log.Printf("Error saving preview %s: %v", previewPath, err)
			removeFiles(originalPath, previewPath)
			http.Error(w, "Preview generation failed", http.StatusInternalServerError)
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

	// Carry over immutable/user-set fields from the previous record so that
	// a re-upload does not silently drop IsPinned, PinnedAt, or Category.
	var isPinned bool
	var pinnedAt int64
	var category string
	if oldWp != nil {
		isPinned = oldWp.IsPinned
		pinnedAt = oldWp.PinnedAt
		category = oldWp.Category
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
		ImagePath:   originalPath,
		PreviewPath: previewPath,
	}
	storage.Global.Set(linkName, wp)
	if err := storage.Global.Save(); err != nil {
		log.Printf("Error saving after upload: %v — rolling back", err)
		storage.Global.Delete(linkName)
		removeFiles(originalPath, previewPath)
		http.Error(w, "Failed to persist upload", http.StatusInternalServerError)
		return
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

func saveImage(img image.Image, format, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	encodeErr := encodeImage(out, img, format)
	closeErr := out.Close()
	if encodeErr != nil {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			log.Printf("Error removing partial file %s: %v", path, removeErr)
		}
		return encodeErr
	}
	return closeErr
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
		log.Printf("[SECURITY] Rejected local image %s: %v", path, dimErr)
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

	// SSRF protection is handled entirely by ssrfSafeDialer in the transport.
	// No pre-flight DNS check here to avoid consuming the request context timeout.

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	// Transport carries its own ResponseHeaderTimeout; rely on the context
	// for the overall deadline instead of duplicating it in Client.Timeout.
	client := &http.Client{
		Transport: getTransport(),
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("downloadImage: network error fetching %s: %v", urlStr, err)
		return nil, "", nil, errors.New("network error")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
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

	if dimErr := checkImageDimensions(bytes.NewReader(buf)); dimErr != nil {
		log.Printf("[SECURITY] Rejected remote image %s: %v", urlStr, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}

	mimeType := http.DetectContentType(buf)
	ext, ok := mimeToExt[mimeType]
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
