package config

// Image processing constants
const (
	// MaxImageDimension is the maximum allowed image width or height.
	// Images exceeding this size are rejected to prevent decompression bombs.
	MaxImageDimension = 16384 // 16K pixels

	// ThumbnailMaxWidth is the maximum width for generated thumbnails.
	ThumbnailMaxWidth = 200

	// ThumbnailMaxHeight is the maximum height for generated thumbnails.
	ThumbnailMaxHeight = 160

	// JPEGQuality is the quality setting for JPEG encoding (1-100).
	JPEGQuality = 85

	// WebPQuality is the quality setting for WebP encoding (1-100).
	WebPQuality = 85

	// GIFColors is the number of colors in GIF palette.
	GIFColors = 256

	// DefaultCompressionQuality is the default client-side compression quality (1-100).
	// 85 = good balance between quality and file size.
	// 100 = maximum quality, no compression.
	DefaultCompressionQuality = 85

	// DefaultCompressionScale is the default client-side scale percentage (1-100).
	// 100 = full size (1920x1080 max), 50 = half size (960x540 max).
	DefaultCompressionScale = 100
)

// Validation constants
const (
	// MinUploadMB is the minimum allowed MaxUploadMB value.
	MinUploadMB = 1

	// DefaultMaxUploadMB is the default upload size limit in megabytes.
	DefaultMaxUploadMB = 50

	// DefaultMaxConcurrentUploads is the default number of concurrent uploads.
	DefaultMaxConcurrentUploads = 2
)

// Network constants
const (
	// DownloadTimeout is the maximum time allowed for downloading remote images.
	// Set to 90 seconds to accommodate slow connections and large files.
	DownloadTimeout = 90 // seconds

	// HTTPReadTimeout covers request headers and body reading.
	HTTPReadTimeout = 30 // seconds

	// HTTPWriteTimeout must exceed DownloadTimeout to allow large file responses.
	HTTPWriteTimeout = 120 // seconds

	// HTTPIdleTimeout is the maximum time to wait for the next request.
	HTTPIdleTimeout = 120 // seconds

	// ShutdownTimeout is the graceful shutdown timeout.
	ShutdownTimeout = 30 // seconds
)

// Rate limiting constants
const (
	// DefaultPublicRatePerMin is the default rate limit for public endpoints.
	DefaultPublicRatePerMin = 120

	// DefaultUploadRatePerMin is the default rate limit for upload endpoint.
	DefaultUploadRatePerMin = 20

	// DefaultRateBurst is the default burst allowance for rate limiting.
	DefaultRateBurst = 10

	// RateLimitCleanerInterval is the sweep period for idle rate-limit entries.
	RateLimitCleanerInterval = 120 // seconds (2 minutes)
)

// File system constants
const (
	// DefaultMaxWalkDepth is the default maximum directory recursion depth for external images.
	// Can be overridden via MAX_WALK_DEPTH environment variable.
	DefaultMaxWalkDepth = 3

	// FileCopyBufferSize is the buffer size for copying files (1MB).
	// Larger buffer improves performance for large video files.
	FileCopyBufferSize = 1024 * 1024
)
