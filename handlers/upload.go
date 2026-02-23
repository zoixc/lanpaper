package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
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
	url_ "net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chai2010/webp"
	xdraw "golang.org/x/image/draw"
	"lanpaper/config"
	"lanpaper/storage"
	"lanpaper/utils"
)

var uploadSem chan struct{}

func InitUploadSemaphore(maxConcurrent int) {
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	uploadSem = make(chan struct{}, maxConcurrent)
}

// httpTransport is a shared transport so connections are pooled across downloads.
// It is rebuilt lazily when proxy/TLS settings change (guarded by transportMu).
var (
	transportMu      sync.Mutex
	cachedTransport  *http.Transport
	cachedProxyHost  string
	cachedInsecure   bool
)

func getTransport() *http.Transport {
	transportMu.Lock()
	defer transportMu.Unlock()

	proxyHost := config.Current.ProxyHost
	insecure := config.Current.InsecureSkipVerify

	// Rebuild only when relevant settings differ from last build
	if cachedTransport != nil && cachedProxyHost == proxyHost && cachedInsecure == insecure {
		return cachedTransport
	}

	t := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: insecure},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		MaxIdleConns:          20,
		MaxIdleConnsPerHost:   5,
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}

	if proxyHost != "" {
		proxyURL := &url_.URL{
			Scheme: config.Current.ProxyType,
			Host:   net.JoinHostPort(proxyHost, config.Current.ProxyPort),
		}
		if config.Current.ProxyUsername != "" {
			proxyURL.User = url_.UserPassword(config.Current.ProxyUsername, config.Current.ProxyPassword)
		}
		t.Proxy = http.ProxyURL(proxyURL)
	}

	cachedTransport = t
	cachedProxyHost = proxyHost
	cachedInsecure = insecure
	return t
}

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

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Error closing destination file %s: %v", dst, cerr)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}
	return nil
}

func copyVideoFromReader(r io.Reader, dst string) error {
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

// thumbnail resizes src to fit within maxW x maxH using golang.org/x/image/draw
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
			http.Error(w, "Failed to load image: "+err.Error(), http.StatusBadRequest)
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
			http.Error(w, "Unsupported file type: "+contentType, http.StatusBadRequest)
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
			img, _, err = image.Decode(uploadedFile)
			if err != nil {
				log.Printf("Image decode error for %s: %v", safeFilename, err)
				http.Error(w, "Invalid image decode", http.StatusBadRequest)
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

	originalPath := filepath.Join("static", "images", linkName+"."+ext)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")

	if isVideo {
		if urlStr == "" {
			if _, err := uploadedFile.Seek(0, io.SeekStart); err != nil {
				log.Printf("Error seeking file for video copy: %v", err)
				http.Error(w, "Failed to prepare video file", http.StatusInternalServerError)
				return
			}
			if err := copyVideoFromReader(uploadedFile, originalPath); err != nil {
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
				http.Error(w, "Failed to copy video from external source", http.StatusInternalServerError)
				return
			}
		}
		previewPath = ""

	} else {
		if err := saveImage(img, ext, originalPath); err != nil {
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
		ImageURL:    "/static/images/" + linkName + "." + ext,
		Preview:     previewURL,
		HasImage:    true,
		MIMEType:    ext,
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

	log.Printf("Uploaded: %s (%s, %d KB)", linkName, ext, fi.Size()/1024)
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
		return nil, "", nil, fmt.Errorf("file not found on server")
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
		return nil, "", nil, err
	}

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", nil, err
	}
	if format == "jpeg" {
		format = "jpg"
	}
	return img, format, head, nil
}

func downloadImage(ctx context.Context, urlStr string) (image.Image, string, []byte, error) {
	parsedURL, err := url_.Parse(urlStr)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, "", nil, fmt.Errorf("invalid URL")
	}

	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	client := &http.Client{
		Transport: getTransport(),
		Timeout:   90 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", nil, fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	if resp.ContentLength > 0 && resp.ContentLength > int64(config.Current.MaxUploadMB)<<20 {
		log.Printf("Security: rejected download with Content-Length %d (max: %d)", resp.ContentLength, int64(config.Current.MaxUploadMB)<<20)
		return nil, "", nil, fmt.Errorf("file too large: %d bytes", resp.ContentLength)
	}

	limitReader := io.LimitReader(resp.Body, int64(config.Current.MaxUploadMB)<<20)
	buf, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, "", nil, fmt.Errorf("read error: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", nil, fmt.Errorf("decode error")
	}

	ext := "jpg"
	if ct := resp.Header.Get("Content-Type"); ct != "" {
		if strings.Contains(ct, "png") {
			ext = "png"
		} else if strings.Contains(ct, "gif") {
			ext = "gif"
		} else if strings.Contains(ct, "webp") {
			ext = "webp"
		}
	}
	return img, ext, buf, nil
}
