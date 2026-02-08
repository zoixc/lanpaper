package handlers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	url_ "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chai2010/webp"
	"github.com/nfnt/resize"
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

func Upload(w http.ResponseWriter, r *http.Request) {
	select {
	case uploadSem <- struct{}{}:
		defer func() { <-uploadSem }()
	default:
		http.Error(w, "Too many concurrent uploads", http.StatusTooManyRequests)
		return
	}

	maxBytes := int64(config.Current.MaxUploadMB) << 20
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
		img     image.Image
		ext     string
		err     error
		isVideo bool
	)

	urlStr := r.FormValue("url")

	// Upload logic
	if urlStr != "" {
		if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
			img, ext, err = downloadImage(r.Context(), urlStr)
		} else {
			// Local files from server - enhanced path validation
			if !utils.IsValidLocalPath(urlStr) {
				log.Printf("Security: blocked invalid path attempt: %s", urlStr)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}

			baseDir := config.Current.ExternalImageDir
			if baseDir == "" {
				baseDir = "external/images"
			}

			// Resolve to absolute path and validate
			absBase, err := filepath.Abs(baseDir)
			if err != nil {
				log.Printf("Error resolving base directory: %v", err)
				http.Error(w, "Server configuration error", http.StatusInternalServerError)
				return
			}

			localPath := filepath.Join(absBase, filepath.Clean(urlStr))
			absPath, err := filepath.Abs(localPath)
			if err != nil {
				log.Printf("Error resolving file path: %v", err)
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}

			// Ensure path is within base directory
			if !strings.HasPrefix(absPath, absBase) {
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
				err = nil
			} else {
				img, ext, err = loadLocalImage(absPath)
			}
		}

		if err != nil {
			log.Printf("Image load error for link %s: %v", linkName, err)
			http.Error(w, "Failed to load image: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		// Uploading from client
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			http.Error(w, "No file provided", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Validate file size from header
		if header.Size > maxBytes {
			http.Error(w, "File too large", http.StatusBadRequest)
			return
		}

		head := make([]byte, 512)
		if _, err := file.Read(head); err != nil {
			http.Error(w, "Read error", http.StatusBadRequest)
			return
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			log.Printf("Error seeking file: %v", err)
			http.Error(w, "File seek error", http.StatusInternalServerError)
			return
		}

		contentType := http.DetectContentType(head)
		allowed := map[string]string{
			"image/jpeg": "jpg", "image/png": "png",
			"image/gif": "gif", "image/webp": "webp",
			"image/bmp": "bmp", "image/tiff": "tif",
			"video/mp4": "mp4", "video/webm": "webm",
		}

		if e, ok := allowed[contentType]; ok {
			ext = e
			if ext == "mp4" || ext == "webm" {
				isVideo = true
			}
		} else {
			log.Printf("Rejected unsupported content type: %s", contentType)
			http.Error(w, "Unsupported image format: "+contentType, http.StatusBadRequest)
			return
		}

		if !isVideo {
			img, _, err = image.Decode(file)
			if err != nil {
				http.Error(w, "Invalid image decode", http.StatusBadRequest)
				return
			}
		}
	}

	// Remove old image
	if oldWp != nil && oldWp.HasImage {
		if err := os.Remove(oldWp.ImagePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing old image %s: %v", oldWp.ImagePath, err)
		}
		if err := os.Remove(oldWp.PreviewPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing old preview %s: %v", oldWp.PreviewPath, err)
		}
	}

	// Save new image
	originalPath := filepath.Join("static", "images", linkName+"."+ext)
	previewPath := filepath.Join("static", "images", "previews", linkName+".webp")

	if isVideo {
		// Copy video file
		if urlStr == "" {
			file, _, _ := r.FormFile("file")
			if _, err := file.Seek(0, 0); err != nil {
				log.Printf("Error seeking file for video copy: %v", err)
			}
			out, err := os.Create(originalPath)
			if err != nil {
				log.Printf("Error creating video file %s: %v", originalPath, err)
				http.Error(w, "Failed to save video", http.StatusInternalServerError)
				return
			}
			if _, err := io.Copy(out, file); err != nil {
				log.Printf("Error copying video: %v", err)
			}
			out.Close()
			file.Close()
		} else if !strings.HasPrefix(urlStr, "http") {
			// Already validated path above
			baseDir := config.Current.ExternalImageDir
			if baseDir == "" {
				baseDir = "external/images"
			}
			absBase, _ := filepath.Abs(baseDir)
			srcPath := filepath.Join(absBase, filepath.Clean(urlStr))

			in, err := os.Open(srcPath)
			if err == nil {
				out, err := os.Create(originalPath)
				if err != nil {
					log.Printf("Error creating video file %s: %v", originalPath, err)
				} else {
					if _, err := io.Copy(out, in); err != nil {
						log.Printf("Error copying video from external: %v", err)
					}
					out.Close()
				}
				in.Close()
			} else {
				log.Printf("Error opening external video %s: %v", srcPath, err)
			}
		}
		previewPath = "/static/icons/video-placeholder.png"

	} else {
		// Save & pic resize
		if err := saveImage(img, ext, originalPath); err != nil {
			log.Printf("Error saving image %s: %v", originalPath, err)
			http.Error(w, "Save failed", http.StatusInternalServerError)
			return
		}

		thumb := resize.Thumbnail(200, 160, img, resize.Bilinear)
		if err := saveImage(thumb, "webp", previewPath); err != nil {
			log.Printf("Error saving preview %s: %v", previewPath, err)
			if err := os.Remove(originalPath); err != nil {
				log.Printf("Error removing original after preview fail: %v", err)
			}
			http.Error(w, "Preview failed", http.StatusInternalServerError)
			return
		}
		previewPath = "/static/images/previews/" + linkName + ".webp"
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

	wp := &storage.Wallpaper{
		ID:          linkName,
		LinkName:    linkName,
		ImageURL:    "/static/images/" + linkName + "." + ext,
		Preview:     previewPath,
		HasImage:    true,
		MIMEType:    ext,
		SizeBytes:   fi.Size(),
		ModTime:     fi.ModTime().Unix(),
		CreatedAt:   createdAt,
		ImagePath:   originalPath,
		PreviewPath: previewPath,
	}

	if isVideo {
		wp.PreviewPath = ""
	} else {
		wp.PreviewPath = filepath.Join("static", "images", "previews", linkName+".webp")
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
	defer out.Close()

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

func loadLocalImage(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("file not found on server")
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", err
	}
	if format == "jpeg" {
		format = "jpg"
	}
	return img, format, nil
}

func downloadImage(ctx context.Context, urlStr string) (image.Image, string, error) {
	parsedURL, err := url_.Parse(urlStr)
	if err != nil || !parsedURL.IsAbs() || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, "", fmt.Errorf("invalid URL")
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: config.Current.InsecureSkipVerify},
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	if config.Current.ProxyHost != "" {
		proxyURL := &url_.URL{
			Scheme: config.Current.ProxyType,
			Host:   net.JoinHostPort(config.Current.ProxyHost, config.Current.ProxyPort),
		}
		if config.Current.ProxyUsername != "" {
			proxyURL.User = url_.UserPassword(config.Current.ProxyUsername, config.Current.ProxyPassword)
		}
		transport.Proxy = http.ProxyURL(proxyURL)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Lanpaper/1.0)")
	req.Header.Set("Accept", "image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("network error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	limitReader := io.LimitReader(resp.Body, int64(config.Current.MaxUploadMB)<<20)
	buf, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, "", fmt.Errorf("read error: %v", err)
	}

	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, "", fmt.Errorf("decode error")
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
	return img, ext, nil
}
