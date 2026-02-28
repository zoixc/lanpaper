package config

const (
	MaxImageDimension = 16384 // max width/height in pixels; prevents decompression bombs
	ThumbnailMaxWidth  = 200
	ThumbnailMaxHeight = 160
	JPEGQuality        = 85
	WebPQuality        = 85
	GIFColors          = 256

	DefaultCompressionQuality = 85
	DefaultCompressionScale   = 100
)

const (
	MinUploadMB             = 1
	DefaultMaxUploadMB      = 50
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
	DefaultMaxWalkDepth  = 3
	FileCopyBufferSize   = 1024 * 1024 // 1 MB
)

// ValidCategories is the canonical set of user-assignable category names.
// Add new categories here — handler validation picks them up automatically.
var ValidCategories = map[string]bool{
	"tech": true, "life": true, "work": true, "other": true,
}
