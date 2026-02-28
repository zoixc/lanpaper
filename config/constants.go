package config

const (
	MaxImageDimension  = 16384 // max width/height in pixels; prevents decompression bombs
	ThumbnailMaxWidth  = 640
	ThumbnailMaxHeight = 360
	// DefaultCompressionQuality is the canonical JPEG/WebP quality used everywhere.
	// JPEGQuality and WebPQuality are aliases kept for readability at call sites.
	DefaultCompressionQuality = 85
	JPEGQuality               = DefaultCompressionQuality
	WebPQuality               = DefaultCompressionQuality
	GIFColors                 = 256

	DefaultCompressionScale = 100
)

const (
	MinUploadMB                 = 1
	DefaultMaxUploadMB          = 50
	DefaultMaxConcurrentUploads = 2
)

const (
	DownloadTimeout  = 90  // seconds
	HTTPReadTimeout  = 30  // seconds
	HTTPWriteTimeout = 120 // seconds; must exceed DownloadTimeout
	HTTPIdleTimeout  = 120 // seconds
	ShutdownTimeout  = 30  // seconds
)

const (
	DefaultPublicRatePerMin  = 120
	DefaultUploadRatePerMin  = 20
	DefaultRateBurst         = 10
	RateLimitCleanerInterval = 120 // seconds
)

const (
	DefaultMaxWalkDepth = 3
	FileCopyBufferSize  = 1024 * 1024 // 1 MB
)

// ValidCategories is the canonical set of user-assignable category names.
// Add new categories here — handler validation picks them up automatically.
var ValidCategories = map[string]bool{
	"tech": true, "life": true, "work": true, "other": true,
}

// AllowedMediaExts is the single source of truth for supported file extensions.
// Used by both the upload handler (MIME detection) and the external image browser.
var AllowedMediaExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".tiff": true, ".tif": true,
	".mp4": true, ".webm": true,
}
