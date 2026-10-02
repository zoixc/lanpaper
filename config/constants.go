package config

const (
	MaxImageDimension         = 16384      // max width/height in pixels
	MaxImagePixels            = 36_000_000 // per image (~8K UHD)
	MaxDecodedPixelsInFlight  = 48_000_000 // across uploads + regeneration, bounds concurrent decoding
	ThumbnailMaxWidth         = 640
	ThumbnailMaxHeight        = 360
	ThumbnailQuality          = 80 // WebP quality of admin-panel previews
	DefaultCompressionQuality = 85
	GIFColors                 = 256
	DefaultCompressionScale   = 100
)

const (
	MinUploadMB                 = 1
	MaxUploadMBLimit            = 512 // bound memory/disk use even with an erroneous config
	DefaultMaxUploadMB          = 50
	DefaultMaxConcurrentUploads = 2
	MaxConcurrentUploadsLimit   = 8
)

const (
	DownloadTimeout       = 90  // seconds
	HTTPReadHeaderTimeout = 10  // seconds
	HTTPReadTimeout       = 30  // seconds
	HTTPWriteTimeout      = 120 // seconds; must exceed DownloadTimeout
	HTTPIdleTimeout       = 120 // seconds
	ShutdownTimeout       = 30  // seconds
	MaxRedirects          = 5   // outbound download redirect cap (SSRF defence)

	// Authenticated uploads may need longer than HTTPReadTimeout on slow
	// links: their deadline is UploadBaseTimeout plus the time needed to
	// transfer the maximum request size at UploadMinBytesPerSec.
	UploadBaseTimeout    = 120       // seconds
	UploadMinBytesPerSec = 256 << 10 // 256 KiB/s
	// RegenerateTimeout bounds a preview regeneration of the whole library.
	RegenerateTimeout = 30 * 60 // seconds
)

const (
	DefaultPublicRatePerMin  = 120
	DefaultUploadRatePerMin  = 20
	DefaultRateBurst         = 10
	RateLimitCleanerInterval = 120 // seconds
)

const (
	DefaultMaxWalkDepth = 3
)

// Media storage lives outside the static web root so access-control on
// public links cannot be bypassed via /static/images/...
const (
	MediaDir    = "data/media"
	PreviewDir  = "data/previews"
	LegacyMedia = "static/images" // pre-migration location
	DataDirPerm = 0o700           // data directories are private to the service user
)

// Access level values for per-link visibility.
const (
	AccessPublic = "public" // anyone on the internet
	AccessLocal  = "local"  // only private/LAN/loopback clients
	AccessToken  = "token"  // requires secret token in query/header
	AccessAuth   = "auth"   // requires admin Basic Auth
)

// ValidAccessLevels is the allow-list for AccessLevel values.
var ValidAccessLevels = map[string]bool{
	AccessPublic: true,
	AccessLocal:  true,
	AccessToken:  true,
	AccessAuth:   true,
}

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

// IsVideoExt reports whether a stored media extension is a video format.
func IsVideoExt(ext string) bool { return ext == "mp4" || ext == "webm" }

// Version history and playlist items live in their own directories next to
// the media they belong to, so the canonical data/media/{link}.{ext} layout
// (and every tool that reads it) keeps working unchanged.
const (
	HistoryDir = "data/history" // data/history/{link}/{version}.{ext}
	ItemsDir   = "data/items"   // data/items/{link}/{id}.{ext}
)

// History defaults and hard bounds. HISTORY_LIMIT=0 disables versioning
// entirely; HISTORY_MAX_MB=0 disables the global budget.
const (
	DefaultHistoryLimit = 3
	MaxHistoryLimit     = 50
	DefaultHistoryMaxMB = 512
	MaxHistoryMaxMB     = 1 << 16 // 65536 MiB = 64 GiB, i.e. "no realistic limit"
)

// Playlist defaults and hard bounds. Rotation intervals are clamped so a
// typo cannot turn a public link into a per-request randomizer.
const (
	DefaultPlaylistMax    = 8
	MaxPlaylistItems      = 64
	DefaultRotateInterval = 60 // seconds
	MinRotateInterval     = 5
	MaxRotateInterval     = 24 * 3600
)

// Rotation orders for RotateConfig.Order.
const (
	RotateOrderSequential = "sequential"
	RotateOrderRandom     = "random"
)

// Publish keys (PUBLISH_KEYS) authorize uploads without admin credentials.
// They are secrets, so they are only ever compared as SHA-256 digests and
// only ever logged as an 8 hex character fingerprint.
const (
	MaxPublishKeys    = 32
	MinPublishKeyLen  = 16
	KeyFingerprintLen = 8
)
