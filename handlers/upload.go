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

// InitUploadSemaphore sets the maximum number of concurrent uploads.
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

// getTransport returns a cached *http.Transport, rebuilding when proxy/TLS settings change.
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
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure}, //nolint:gosec
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

// ssrfSafeDialer resolves DNS and pins to the first IP, blocking private/internal addresses.
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
	// Pin to the first public IP to prevent DNS rebinding
	var safeIP string
	for _, ipAddr := range ips {
		ip := ipAddr.IP
		if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			continue // skip private
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
		return nil, fmt.Errorf("SSRF: blocked %s (no safe public IP found)", host)
	}

	return d.inner.DialContext(ctx, network, net.JoinHostPort(safeIP, port))
}

// copyVideoFile streams src into dst with a buffered writer.
// Pass a non-nil r to stream from a reader; otherwise srcPath is opened.
func copyVideoFile(srcPath, dst string, r io.Reader) error {
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

// storedExt returns the on-disk extension; BMP/TIFF are re-encoded as JPEG.
func storedExt(ext string) string {
	if ext == "bmp" || ext == "tiff" {
		return "jpg"
	}
	return ext
}

func checkImageDimensions(r io.ReadSeeker) error {
	cfg, _, err := image.DecodeConfig(r)
	if err != nil {
		return nil // let the full decode surface the real error
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
			// ssrfSafeDialer handles SSRF prevention with DNS pinning at connection time.
			img, ext, fileData, err = downloadImage(r.Context(), urlStr)
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
			http.Error(w, err.Error(), http.StatusBadRequest)
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

	saveExt := storedExt(ext)
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
			copyErr = copyVideoFile("", originalPath, upFile)
		} else if !strings.HasPrefix(urlStr, "http") {
			absPath, _, pathErr := utils.ValidateAndResolvePath(utils.ExternalBaseDir(), urlStr)
			if pathErr != nil {
				log.Printf("Security: path validation failed for video %s: %v", urlStr, pathErr)
				http.Error(w, "Path outside allowed directory", http.StatusForbidden)
				return
			}
			copyErr = copyVideoFile(absPath, originalPath, nil)
		}
		if copyErr != nil {
			log.Printf("Error saving video %s: %v", originalPath, copyErr)
			http.Error(w, "Failed to save video", http.StatusInternalServerError)
			return
		}
		previewPath = ""
	} else {
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
		storage.Global.Delete(linkName)
		removeFiles(originalPath, previewPath)
		http.Error(w, "Failed to persist upload", http.StatusInternalServerError)
		return
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

// saveImage encodes img to path; removes the file on encode error.
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
	switch format {
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: config.JPEGQuality})
	case "png":
		return png.Encode(w, img)
	case "gif":
		return gif.Encode(w, img, &gif.Options{NumColors: config.GIFColors})
	case "webp":
		return webp.Encode(w, img, &webp.Options{Quality: config.WebPQuality})
	default:
		return jpeg.Encode(w, img, &jpeg.Options{Quality: config.JPEGQuality})
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
		log.Printf("Security: oversized local image %s: %v", path, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, "", nil, fmt.Errorf("seek: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", nil, err
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

	// Read one byte more than maxBytes to properly detect if file exceeds limit
	buf, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", nil, errors.New("read error")
	}
	if int64(len(buf)) > maxBytes {
		log.Printf("Security: rejected download body > %d bytes", maxBytes)
		return nil, "", nil, errors.New("file too large")
	}

	if dimErr := checkImageDimensions(bytes.NewReader(buf)); dimErr != nil {
		log.Printf("Security: oversized remote image %s: %v", urlStr, dimErr)
		return nil, "", nil, errors.New("image dimensions too large")
	}
	// Use decoder-reported format, not Content-Type — CDNs often send application/octet-stream.
	img, format, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", nil, errors.New("invalid or unsupported image format")
	}
	return img, normalizeFormat(format), buf, nil
}
