package handlers

import (
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

	// Register additional image decoders into the image.Decode registry.
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"

	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

func init() {
	// Register the WebP decoder so image.Decode handles WebP input.
	image.RegisterFormat("webp", "RIFF????WEBP", webp.Decode, webp.DecodeConfig)
}

var uploadSem chan struct{}

func InitUploadSemaphore(maxConcurrent int) {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	uploadSem = make(chan struct{}, maxConcurrent)
}

var (
	transportMu     sync.Mutex
	cachedTransport *http.Transport
	cachedProxyHost string
	cachedInsecure  bool
)

// getTransport returns a cached *http.Transport, rebuilding it when proxy or
// TLS settings have changed.
func getTransport() *http.Transport {
	transportMu.Lock()
	defer transportMu.Unlock()

	proxyHost := config.Current.ProxyHost
	insecure := config.Current.InsecureSkipVerify

	if cachedTransport != nil && cachedProxyHost == proxyHost && cachedInsecure == insecure {
		return cachedTransport
	}

	t := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
		// SSRF-safe dialer: resolves DNS and blocks private IPs.
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

	cachedTransport = t
	cachedProxyHost = proxyHost
	cachedInsecure = insecure
	return t
}

// ssrfSafeDialer wraps net.Dialer and blocks connections to private/internal IPs.
type ssrfSafeDialer struct {
	inner *net.Dialer
}

func (d *ssrfSafeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, fmt.Errorf("DNS resolution failed for %s", host)
	}

	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return nil, fmt.Errorf("SSRF: connection to %s (%s) is blocked", host, ip)
		}
		for _, cidr := range utils.PrivateRanges() {
			if cidr.Contains(ip) {
				return nil, fmt.Errorf("SSRF: connection to %s (%s) is blocked", host, ip)
			}
		}
	}

	// Pin to the first resolved IP to prevent DNS rebinding attacks.
	resolvedAddr := net.JoinHostPort(ips[0].IP.String(), port)
	return d.inner.DialContext(ctx, network, resolvedAddr)
}

// copyVideoToFile copies from r into a new file at dst.
func copyVideoToFile(r io.Reader, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Error closing destination file %s: %v", dst, cerr)
		}
	}()
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}
	return nil
}

// copyVideoFile copies a video from src path to dst path.
func copyVideoFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			log.Printf("Error closing source file %s: %v", src, cerr)
		}
	}()
	return copyVideoToFile(in, dst)
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

// normalizeFormat maps image.Decode format names to internal extensions.
func normalizeFormat(format string) string {
	switch format {
	case "jpeg":
		return "jpg"
	default:
		return format
	}
}

// storedExt returns the extension written to disk.
// bmp and tiff are re-encoded as JPEG, so their stored extension is "jpg".
func storedExt(ext string) string {
	if ext == "bmp" || ext == "tiff" {
		return "jpg"
	}
	return ext
}

// maxImageDimension is the maximum allowed image width or height.
// Larger images are rejected before decoding to prevent decompression bombs.
// 16384 px covers all practical wallpaper sizes (8K = 7680 px wide).
const maxImageDimension = 16384

// checkImageDimensions peeks at the image config without a full decode and
// returns an error if either dimension exceeds maxImageDimension.
func checkImageDimensions(r io.ReadSeeker) error {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil // let the full decode fail with its own error
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return fmt.Errorf("image dimensions %dx%d exceed maximum allowed %dx%d",
			cfg.Width, cfg.Height, maxImageDimension, maxImageDimension)
	}
	return nil
}

// thumbnail resizes src to fit within maxW×maxH using bilinear scaling.
func thumbnail(src image.Image, maxW, maxH int) image.Image {
	srcB := src.Bounds()
	scaleX := float64(maxW) / float64(srcB.Dx())
	scaleY := float64(maxH) / float64(srcB.Dy())
	scale := scaleX
	if scaleY < scale {
		scale = scaleY
	}
	if scale >= 1 {
		return src
	}
	newW := int(float64(srcB.Dx()) * scale)
	newH := int(float64(srcB.Dy()) * scale)
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	xdraw.BiLinear.Scale(dst, dst.Bounds(), src, srcB, draw.Over, nil)
	return dst
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
		log.Printf("Security: rejected upload with Content-Length %d (max: %d)", r.ContentLength, maxBytes)
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
		isVideo      bool
		fileData     []byte
		uploadedFile multipart.File
	)

	urlStr := r.FormValue("url")

	if urlStr != "" {
		if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
			parsedURL, parseErr := url.Parse(urlStr)
			if parseErr != nil {
				http.Error(w, "Invalid URL", http.StatusBadRequest)
				return
			}
			if err := utils.ValidateRemoteURL(parsedURL.Hostname()); err != nil {
				log.Printf("Security: blocked SSRF attempt to %s", parsedURL.Hostname())
				http.Error(w, "URL not allowed", http.StatusForbidden)
				return
			}
			img, ext, fileData, err = downloadImage(r.Context(), urlStr)
		} else {
			if !utils.IsValidLocalPath(urlStr) {
				log.Printf("Security: blocked invalid path attempt: %s", urlStr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}

			baseDir := config.Current.ExternalImageDir
			if baseDir == "" {
				baseDir = "external/images"
			}

			absBase, absErr := filepath.Abs(baseDir)
			if absErr != nil {
				log.Printf("Error resolving base directory: %v", absErr)
				http.Error(w, "Server configuration error", http.StatusInternalServerError)
				return
			}

			localPath := filepath.Join(absBase, filepath.Clean(urlStr))
			absPath, absErr := filepath.Abs(localPath)
			if absErr != nil {
				log.Printf("Error resolving file path: %v", absErr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}

			if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
				log.Printf("Security: blocked path traversal attempt: %s -> %s", urlStr, absPath)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}

			// Resolve symlinks to prevent escape to arbitrary paths.
			realPath, realErr := filepath.EvalSymlinks(absPath)
			if realErr != nil {
				log.Printf("Security: cannot resolve symlink %s: %v", absPath, realErr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}
			if !strings.HasPrefix(realPath, absBase+string(filepath.Separator)) && realPath != absBase {
				log.Printf("Security: blocked symlink escape: %s -> %s", absPath, realPath)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}

			ext = strings.ToLower(filepath.Ext(absPath))
			if len(ext) > 1 {
				ext = ext[1:]
			}
			if ext == "mp4" || ext == "webm" {
				isVideo = true
			} else {
				img, ext, fileData, err = loadLocalImage(absPath)
			}
		}

		if err != nil {
			log.Printf("Image load error for link %s: %v", linkName, err)
			http.Error(w, "Failed to load image", http.StatusBadRequest)
			return
		}
	} else {
		var header *multipart.FileHeader
		var ferr error
		uploadedFile, header, ferr = r.FormFile("file")
		if ferr != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer uploadedFile.Close()

		if header.Size > maxBytes {
			log.Printf("Security: rejected file %s with size %d (max: %d)", header.Filename, header.Size, maxBytes)
			http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
			return
		}

		// SanitizeFilename is used only for safe log output; actual storage uses linkName.
		safeFilename := utils.SanitizeFilename(header.Filename)

		head := make([]byte, 512)
		n, readErr := uploadedFile.Read(head)
		if readErr != nil && readErr != io.EOF {
			http.Error(w, "Read error", http.StatusBadRequest)
			return
		}
		head = head[:n]
		if _, err := uploadedFile.Seek(0, io.SeekStart); err != nil {
			log.Printf("Error seeking file: %v", err)
			http.Error(w, "File seek error", http.StatusInternalServerError)
			return
		}

		contentType := http.DetectContentType(head)
		e, ok := mimeToExt[contentType]
		if !ok {
			log.Printf("Security: rejected file %s with unsupported MIME type: %s", safeFilename, contentType)
			http.Error(w, "Unsupported file type", http.StatusBadRequest)
			return
		}
		ext = e
		if ext == "mp4" || ext == "webm" {
			isVideo = true
		}

		if err := utils.ValidateFileType(head, ext); err != nil {
			log.Printf("Security: magic bytes validation failed for %s: %v", safeFilename, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}

		if !isVideo {
			if dimErr := checkImageDimensions(uploadedFile); dimErr != nil {
				log.Printf("Security: rejected oversized image from %s: %v", safeFilename, dimErr)
				http.Error(w, "Image dimensions too large", http.StatusBadRequest)
				return
			}
			if _, err := uploadedFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Error seeking file after dimension check: %v", err)
				http.Error(w, "File seek error", http.StatusInternalServerError)
				return
			}

			var decodeErr error
			img, _, decodeErr = image.Decode(uploadedFile)
			if decodeErr != nil {
				log.Printf("Image decode error for %s: %v", safeFilename, decodeErr)
				http.Error(w, "Invalid image", http.StatusBadRequest)
				return
			}
		}
	}

	if len(fileData) > 0 && !isVideo {
		if err := utils.ValidateFileType(fileData, ext); err != nil {
			log.Printf("Security: magic bytes validation failed for link %s: %v", linkName, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}
	}

	if oldWp != nil && oldWp.HasImage {
		if err := os.Remove(oldWp.ImagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing old image %s: %v", oldWp.ImagePath, err)
		}
		if oldWp.PreviewPath != "" {
			if err := os.Remove(oldWp.PreviewPath); err != nil && !os.IsNotExist(err) {
				log.Printf("Error removing old preview %s: %v", oldWp.PreviewPath, err)
			}
		}
	}

	// bmp/tiff are re-encoded as JPEG, so the on-disk extension is "jpg".
	saveExt := storedExt(ext)
	originalPath := filepath.Join("static", "images", linkName+"."+saveExt)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")

	if isVideo {
		if urlStr == "" {
			if _, err := uploadedFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Error seeking file for video copy: %v", err)
				http.Error(w, "Failed to prepare video file", http.StatusInternalServerError)
				return
			}
			if err := copyVideoToFile(uploadedFile, originalPath); err != nil {
				log.Printf("Error copying uploaded video to %s: %v", originalPath, err)
				http.Error(w, "Failed to save video", http.StatusInternalServerError)
				return
			}
		} else if !strings.HasPrefix(urlStr, "http") {
			baseDir := config.Current.ExternalImageDir
			if baseDir == "" {
				baseDir = "external/images"
			}
			absBase, _ := filepath.Abs(baseDir)
			srcPath := filepath.Join(absBase, filepath.Clean(urlStr))
			if err := copyVideoFile(srcPath, originalPath); err != nil {
				log.Printf("Error copying external video %s to %s: %v", srcPath, originalPath, err)
				http.Error(w, "Failed to copy video", http.StatusInternalServerError)
				return
			}
		}
		previewPath = ""
	} else {
		if err := saveImage(img, saveExt, originalPath); err != nil {
			log.Printf("Error saving image %s: %v", originalPath, err)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}

		thumb := thumbnail(img, 200, 160)
		if err := saveImage(thumb, "webp", previewPath); err != nil {
			log.Printf("Error saving preview %s: %v", previewPath, err)
			if removeErr := os.Remove(originalPath); removeErr != nil && !os.IsNotExist(removeErr) {
				log.Printf("Error removing original after preview fail: %v", removeErr)
			}
			http.Error(w, "Preview generation failed", http.StatusInternalServerError)
			return
		}
	}

	fi, err := os.Stat(originalPath)
	if err != nil {
		log.Printf("Error stating uploaded file %s: %v", originalPath, err)
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
		log.Printf("Error saving wallpapers after upload: %v", err)
	}

	if config.Current.MaxImages > 0 {
		go func() { storage.PruneOldImages(config.Current.MaxImages) }()
	}

	log.Printf("Uploaded: %s (%s, %d KB)", linkName, saveExt, fi.Size()/1024)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(wp); err != nil {
		log.Printf("Error encoding upload response: %v", err)
	}
}

func saveImage(img image.Image, format, path string) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create image file: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Error closing file %s: %v", path, cerr)
		}
	}()

	switch format {
	case "jpg", "jpeg":
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 85})
	case "png":
		return png.Encode(out, img)
	case "gif":
		return gif.Encode(out, img, &gif.Options{NumColors: 256})
	case "webp":
		return webp.Encode(out, img, &webp.Options{Quality: 85})
	default:
		return jpeg.Encode(out, img, &jpeg.Options{Quality: 85})
	}
}

func loadLocalImage(path string) (image.Image, string, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", nil, errors.New("file not found")
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			log.Printf("Error closing file %s: %v", path, cerr)
		}
	}()

	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, errors.New("failed to read file")
	}

	if dimErr := checkImageDimensions(f); dimErr != nil {
		log.Printf("Security: rejected oversized local image %s: %v", path, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, errors.New("failed to read file")
	}

	img, format, err := image.Decode(f)
	if err != nil {
		log.Printf("Image decode error for %s: %v", path, err)
		return nil, "", nil, errors.New("invalid or unsupported image format")
	}
	return img, normalizeFormat(format), head, nil
}

func downloadImage(ctx context.Context, urlStr string) (image.Image, string, []byte, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, "", nil, fmt.Errorf("invalid URL")
	}

	// One context timeout governs the entire download.
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client := &http.Client{Transport: getTransport()}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("network error")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		log.Printf("Security: rejected download with Content-Length %d (max: %d)", resp.ContentLength, maxBytes)
		return nil, "", nil, fmt.Errorf("file too large")
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, "", nil, fmt.Errorf("read error")
	}

	if dimErr := checkImageDimensions(bytes.NewReader(buf)); dimErr != nil {
		log.Printf("Security: rejected oversized remote image from %s: %v", urlStr, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}

	// Use the format reported by the decoder, not Content-Type —
	// CDNs often serve images with application/octet-stream.
	img, format, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", nil, fmt.Errorf("invalid or unsupported image format")
	}

	return img, normalizeFormat(format), buf, nil
}
