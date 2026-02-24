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
	return d.inner.DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
}

// copyVideoToFile streams r into a new file at dst.
func copyVideoToFile(r io.Reader, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Error closing %s: %v", dst, cerr)
		}
	}()
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}

// copyVideoFile copies a video from src to dst.
func copyVideoFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			log.Printf("Error closing %s: %v", src, cerr)
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

func normalizeFormat(format string) string {
	if format == "jpeg" {
		return "jpg"
	}
	return format
}

// storedExt returns the on-disk extension.
// bmp and tiff are re-encoded as JPEG, so their stored extension is "jpg".
func storedExt(ext string) string {
	if ext == "bmp" || ext == "tiff" {
		return "jpg"
	}
	return ext
}

// maxImageDimension is the maximum allowed image width or height (decompression bomb guard).
const maxImageDimension = 16384

func checkImageDimensions(r io.ReadSeeker) error {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil // let the full decode produce the real error
	}
	if cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
		return fmt.Errorf("image %dx%d exceeds %dx%d limit", cfg.Width, cfg.Height, maxImageDimension, maxImageDimension)
	}
	return nil
}

// thumbnail resizes src to fit within maxW×maxH using bilinear scaling.
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

func isVideo(ext string) bool { return ext == "mp4" || ext == "webm" }

func externalBase() string {
	if d := config.Current.ExternalImageDir; d != "" {
		return d
	}
	return "external/images"
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
	oldWp, exists := storage.Global.Get(linkName)
	if !exists {
		http.Error(w, "Link does not exist", http.StatusBadRequest)
		return
	}

	var (
		img      image.Image
		ext      string
		err      error
		video    bool
		fileData []byte
		upFile   multipart.File
	)

	urlStr := r.FormValue("url")

	if urlStr != "" {
		if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
			parsed, parseErr := url.Parse(urlStr)
			if parseErr != nil {
				http.Error(w, "Invalid URL", http.StatusBadRequest)
				return
			}
			if err := utils.ValidateRemoteURL(parsed.Hostname()); err != nil {
				log.Printf("Security: blocked SSRF attempt to %s", parsed.Hostname())
				http.Error(w, "URL not allowed", http.StatusForbidden)
				return
			}
			img, ext, fileData, err = downloadImage(r.Context(), urlStr)
		} else {
			if !utils.IsValidLocalPath(urlStr) {
				log.Printf("Security: blocked invalid path: %s", urlStr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}
			absBase, absErr := filepath.Abs(externalBase())
			if absErr != nil {
				log.Printf("Error resolving base dir: %v", absErr)
				http.Error(w, "Server configuration error", http.StatusInternalServerError)
				return
			}
			absPath, absErr := filepath.Abs(filepath.Join(absBase, filepath.Clean(urlStr)))
			if absErr != nil {
				log.Printf("Error resolving file path: %v", absErr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}
			if !strings.HasPrefix(absPath, absBase+string(filepath.Separator)) && absPath != absBase {
				log.Printf("Security: path traversal: %s -> %s", urlStr, absPath)
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
				log.Printf("Security: symlink escape: %s -> %s", absPath, realPath)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}
			ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(absPath)), ".")
			if isVideo(ext) {
				video = true
			} else {
				img, ext, fileData, err = loadLocalImage(absPath)
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
				log.Printf("Security: oversized image %s: %v", safeFilename, dimErr)
				http.Error(w, "Image dimensions too large", http.StatusBadRequest)
				return
			}
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error after dimension check: %v", err)
				http.Error(w, "File seek error", http.StatusInternalServerError)
				return
			}
			if img, _, err = image.Decode(upFile); err != nil {
				log.Printf("Image decode error for %s: %v", safeFilename, err)
				http.Error(w, "Invalid image", http.StatusBadRequest)
				return
			}
		}
	}

	if len(fileData) > 0 && !video {
		if err := utils.ValidateFileType(fileData, ext); err != nil {
			log.Printf("Security: magic bytes failed for link %s: %v", linkName, err)
			http.Error(w, "File content does not match file type", http.StatusBadRequest)
			return
		}
	}

	if oldWp != nil && oldWp.HasImage {
		removeFiles(oldWp.ImagePath, oldWp.PreviewPath)
	}

	// bmp/tiff are re-encoded as JPEG; the on-disk extension is "jpg".
	saveExt := storedExt(ext)
	originalPath := filepath.Join("static", "images", linkName+"."+saveExt)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")

	if video {
		if urlStr == "" {
			if _, err := upFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Seek error before video copy: %v", err)
				http.Error(w, "Failed to prepare video file", http.StatusInternalServerError)
				return
			}
			if err := copyVideoToFile(upFile, originalPath); err != nil {
				log.Printf("Error copying video to %s: %v", originalPath, err)
				http.Error(w, "Failed to save video", http.StatusInternalServerError)
				return
			}
		} else if !strings.HasPrefix(urlStr, "http") {
			absBase, _ := filepath.Abs(externalBase())
			if err := copyVideoFile(filepath.Join(absBase, filepath.Clean(urlStr)), originalPath); err != nil {
				log.Printf("Error copying external video to %s: %v", originalPath, err)
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
		if err := saveImage(thumbnail(img, 200, 160), "webp", previewPath); err != nil {
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
		log.Printf("Error saving after upload: %v", err)
	}
	if config.Current.MaxImages > 0 {
		go storage.PruneOldImages(config.Current.MaxImages)
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
		return fmt.Errorf("create: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Error closing %s: %v", path, cerr)
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
			log.Printf("Error closing %s: %v", path, cerr)
		}
	}()

	head := make([]byte, 512)
	n, _ := f.Read(head)
	head = head[:n]

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, fmt.Errorf("seek: %w", err)
	}
	if dimErr := checkImageDimensions(f); dimErr != nil {
		log.Printf("Security: oversized local image %s: %v", path, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, fmt.Errorf("seek: %w", err)
	}

	img, format, err := image.Decode(f)
	if err != nil {
		log.Printf("Image decode error for %s: %v", path, err)
		return nil, "", nil, errors.New("invalid or unsupported image format")
	}
	return img, normalizeFormat(format), head, nil
}

func downloadImage(ctx context.Context, urlStr string) (image.Image, string, []byte, error) {
	parsed, err := url.Parse(urlStr)
	if err != nil || !parsed.IsAbs() || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, "", nil, errors.New("invalid URL")
	}

	// One timeout governs the entire download.
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
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
	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		log.Printf("Security: rejected download Content-Length %d (max %d)", resp.ContentLength, maxBytes)
		return nil, "", nil, errors.New("file too large")
	}

	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	if err != nil {
		return nil, "", nil, errors.New("read error")
	}

	if dimErr := checkImageDimensions(bytes.NewReader(buf)); dimErr != nil {
		log.Printf("Security: oversized remote image %s: %v", urlStr, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}

	// Use the decoder-reported format, not Content-Type — CDNs often send application/octet-stream.
	img, format, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", nil, errors.New("invalid or unsupported image format")
	}
	return img, normalizeFormat(format), buf, nil
}
