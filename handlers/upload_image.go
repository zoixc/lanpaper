// SPDX-License-Identifier: MIT

// Image pipeline for uploads and preview regeneration: type inspection, the
// shared decode budget, scaling and the encoders for stored media and WebP
// previews.
package handlers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/SeriousBug/webp-go-pure/std"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
	_ "golang.org/x/image/tiff"
	xwebp "golang.org/x/image/webp"

	"lanpaper/config"
	"lanpaper/utils"
)

// WebP is explicitly decoded with the streaming x/image/webp decoder below.
// The WebP encoder dependency also registers a decoder that reads the entire
// compressed file into memory; never use image.Decode for WebP uploads.
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

// isoBMFFImageBrands are the major brands of ISO-BMFF files that are still
// images, not video: HEIF (heic/heix/hevc…), AVIF (avif/avis) and the generic
// MIAF brand mif1. They share the ftyp box with MP4 but have no decoder here,
// so an uploaded one used to be stored as .mp4 and served as video/mp4 — a
// tile no browser could play, while the previous build rejected the file.
var isoBMFFImageBrands = map[string]bool{
	"avif": true, "avis": true,
	"heic": true, "heix": true, "heim": true, "heis": true,
	"hevc": true, "hevx": true, "hevm": true, "hevs": true,
	"mif1": true, "msf1": true, "msix": true, "mshf": true,
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
		// An image container is not a video that happens to be unplayable:
		// name it and refuse it, so the file is not stored under a wrong type.
		if brand := strings.ToLower(string(head[8:12])); isoBMFFImageBrands[brand] {
			return "", fmt.Errorf("unsupported image container %q", brand)
		}
		if !isMP4VideoFtyp(head) {
			return "", fmt.Errorf("ftyp box declares no known video brand")
		}
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

// mp4VideoBrands are ftyp brands that mark a video container. A file is stored
// as MP4 only when its major brand or one of its compatible brands is here, so
// an arbitrary file that merely has "ftyp" at offset 4 is refused instead of
// being served as video/mp4.
var mp4VideoBrands = map[string]bool{
	"isom": true, "iso2": true, "iso3": true, "iso4": true, "iso5": true, "iso6": true,
	"mp41": true, "mp42": true, "mp71": true, "avc1": true,
	"M4V ": true, "M4VH": true, "M4VP": true, "dash": true,
	"3gp4": true, "3gp5": true, "3gp6": true, "3g2a": true, "qt  ": true,
}

// isMP4VideoFtyp reports whether head starts with an ftyp box whose major brand
// (offset 8) or compatible brands (from offset 16, up to the box size) include a
// known video brand. The box size must cover at least major and minor brand.
func isMP4VideoFtyp(head []byte) bool {
	if len(head) < 16 {
		return false
	}
	size := int(binary.BigEndian.Uint32(head[0:4]))
	if size < 16 {
		return false
	}
	if mp4VideoBrands[string(head[8:12])] {
		return true
	}
	end := min(size, len(head))
	for off := 16; off+4 <= end; off += 4 {
		if mp4VideoBrands[string(head[off:off+4])] {
			return true
		}
	}
	return false
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

// webpEffort trades WebP encode time for file size (0..9). Preset 3 is the
// fastest one that enables the full lossy search; the default, 0, produces
// previews roughly a third larger for no visible gain.
const webpEffort = 3

func encodeImage(w io.Writer, img image.Image, format string, quality int) error {
	switch format {
	case "jpg", "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "png":
		return png.Encode(w, img)
	case "gif":
		return gif.Encode(w, img, &gif.Options{NumColors: config.GIFColors})
	case "webp":
		return webp.Encode(w, img, &webp.Options{Quality: quality, Effort: webpEffort})
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}
