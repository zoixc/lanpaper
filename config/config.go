package config

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

type RateConfig struct {
	PublicPerMin int `json:"publicPerMin"`
	UploadPerMin int `json:"uploadPerMin"`
	Burst        int `json:"burst"`
}

type CompressionConfig struct {
	Quality int `json:"quality"` // 1-100, JPEG quality
	Scale   int `json:"scale"`   // 1-100, percentage of max dimensions
}

type Config struct {
	Port                 string            `json:"port"`
	MaxUploadMB          int               `json:"maxUploadMB"`
	MaxImages            int               `json:"maxImages"`
	MaxConcurrentUploads int               `json:"maxConcurrentUploads"`
	MaxWalkDepth         int               `json:"maxWalkDepth"`
	ExternalImageDir     string            `json:"externalImageDir"`
	AdminUser            string            `json:"adminUser"`
	AdminPass            string            `json:"adminPass"`
	DisableAuth          bool              `json:"disableAuth,omitempty"`
	InsecureSkipVerify   bool              `json:"insecureSkipVerify,omitempty"`
	ProxyHost            string            `json:"proxyHost,omitempty"`
	ProxyPort            string            `json:"proxyPort,omitempty"`
	ProxyType            string            `json:"proxyType,omitempty"`
	ProxyUsername        string            `json:"proxyUsername,omitempty"`
	ProxyPassword        string            `json:"proxyPassword,omitempty"`
	Rate                 RateConfig        `json:"rate"`
	Compression          CompressionConfig `json:"compression"`
	// TrustedProxy is the IP or CIDR of a reverse proxy in front of Lanpaper.
	// X-Real-IP / X-Forwarded-For are trusted only for requests from this address.
	TrustedProxy string `json:"trustedProxy,omitempty"`
}

var Current Config

// cachedProxy caches the parsed TrustedProxy value set during Load/validate.
// Stored as *parsedProxy via atomic pointer to avoid any lock on the hot path.
type parsedProxy struct {
	ip   *net.IP
	cidr *net.IPNet
}

var cachedProxyPtr atomic.Pointer[parsedProxy]

// Load loads configuration with priority: env vars > config.json > defaults
func Load() {
	// Step 1: Load defaults
	Current = Config{
		Port:                 "8080",
		MaxUploadMB:          DefaultMaxUploadMB,
		MaxConcurrentUploads: DefaultMaxConcurrentUploads,
		MaxWalkDepth:         DefaultMaxWalkDepth,
		ExternalImageDir:     "external/images",
		ProxyType:            "http",
		Rate: RateConfig{
			PublicPerMin: DefaultPublicRatePerMin,
			UploadPerMin: DefaultUploadRatePerMin,
			Burst:        DefaultRateBurst,
		},
		Compression: CompressionConfig{
			Quality: DefaultCompressionQuality,
			Scale:   DefaultCompressionScale,
		},
	}

	// Step 2: Override with config.json (if exists)
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	// Step 3: Override with environment variables (highest priority)
	envString("PORT", &Current.Port)
	envInt("MAX_UPLOAD_MB", &Current.MaxUploadMB)
	envInt("MAX_IMAGES", &Current.MaxImages)
	envInt("MAX_CONCURRENT_UPLOADS", &Current.MaxConcurrentUploads)
	envInt("MAX_WALK_DEPTH", &Current.MaxWalkDepth)
	envString("EXTERNAL_IMAGE_DIR", &Current.ExternalImageDir)
	envString("ADMIN_USER", &Current.AdminUser)
	envString("ADMIN_PASS", &Current.AdminPass)
	envBool("DISABLE_AUTH", &Current.DisableAuth)
	envBool("INSECURE_SKIP_VERIFY", &Current.InsecureSkipVerify)
	envString("PROXY_HOST", &Current.ProxyHost)
	envString("PROXY_PORT", &Current.ProxyPort)
	envString("PROXY_TYPE", &Current.ProxyType)
	// PROXY_USER / PROXY_PASS are accepted as shorter aliases.
	envString("PROXY_USER", &Current.ProxyUsername)
	envString("PROXY_USERNAME", &Current.ProxyUsername)
	envString("PROXY_PASS", &Current.ProxyPassword)
	envString("PROXY_PASSWORD", &Current.ProxyPassword)
	envString("TRUSTED_PROXY", &Current.TrustedProxy)
	envInt("RATE_PUBLIC_PER_MIN", &Current.Rate.PublicPerMin)
	envInt("RATE_UPLOAD_PER_MIN", &Current.Rate.UploadPerMin)
	envInt("RATE_BURST", &Current.Rate.Burst)
	envInt("COMPRESSION_QUALITY", &Current.Compression.Quality)
	envInt("COMPRESSION_SCALE", &Current.Compression.Scale)

	validate()

	mode := "compressed"
	if Current.Compression.Quality == 100 && Current.Compression.Scale == 100 {
		mode = "lossless"
	}
	log.Printf("Config loaded: compression quality=%d scale=%d (%s)",
		Current.Compression.Quality, Current.Compression.Scale, mode)
}

func envString(name string, dst *string) {
	if v := os.Getenv(name); v != "" {
		*dst = v
	}
}

// envInt and envBool never silently drop a typo: an invalid value is logged
// and the previous (config.json or default) value is kept.
func envInt(name string, dst *int) {
	v := os.Getenv(name)
	if v == "" {
		return
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		log.Printf("Warning: ignoring invalid %s=%q (expected an integer)", name, v)
		return
	}
	*dst = n
}

func envBool(name string, dst *bool) {
	v := os.Getenv(name)
	if v == "" {
		return
	}
	b, err := strconv.ParseBool(strings.TrimSpace(v))
	if err != nil {
		log.Printf("Warning: ignoring invalid %s=%q (expected true or false)", name, v)
		return
	}
	*dst = b
}

// parseTrustedProxyValue parses a single TrustedProxy string.
func parseTrustedProxyValue(s string) (*net.IP, *net.IPNet, error) {
	if s == "" {
		return nil, nil, nil
	}
	if ip := net.ParseIP(s); ip != nil {
		return &ip, nil, nil
	}
	_, cidr, err := net.ParseCIDR(s)
	if err != nil {
		return nil, nil, err
	}
	return nil, cidr, nil
}

// IsTrustedProxy reports whether remoteAddr matches the configured TrustedProxy.
func IsTrustedProxy(remoteAddr string) bool {
	p := cachedProxyPtr.Load()
	if p == nil || (p.ip == nil && p.cidr == nil) {
		return false
	}
	host, _, splitErr := net.SplitHostPort(remoteAddr)
	if splitErr != nil {
		host = remoteAddr
	}
	remote := net.ParseIP(host)
	if remote == nil {
		return false
	}
	if p.ip != nil {
		return p.ip.Equal(remote)
	}
	return p.cidr.Contains(remote)
}

// validate clamps out-of-range values to safe defaults. Missing admin
// credentials are deliberately NOT treated as "auth disabled": the admin
// endpoints then fail closed (503) while public links and health checks keep
// working. DISABLE_AUTH=true is the only way to turn authentication off.
func validate() {
	portStr := strings.TrimPrefix(Current.Port, ":")
	if n, err := strconv.Atoi(portStr); err != nil || n < 1 || n > 65535 {
		log.Printf("Warning: invalid port %q, using 8080", Current.Port)
		Current.Port = "8080"
	}

	if Current.MaxUploadMB < MinUploadMB || Current.MaxUploadMB > MaxUploadMBLimit {
		log.Printf("Warning: MaxUploadMB %d out of range (%d-%d), using %d", Current.MaxUploadMB, MinUploadMB, MaxUploadMBLimit, DefaultMaxUploadMB)
		Current.MaxUploadMB = DefaultMaxUploadMB
	}
	if Current.MaxImages < 0 {
		Current.MaxImages = 0
	}
	if Current.MaxConcurrentUploads <= 0 || Current.MaxConcurrentUploads > MaxConcurrentUploadsLimit {
		Current.MaxConcurrentUploads = DefaultMaxConcurrentUploads
	}
	if Current.MaxWalkDepth <= 0 || Current.MaxWalkDepth > 10 {
		log.Printf("Warning: MaxWalkDepth %d out of range (1-10), using %d", Current.MaxWalkDepth, DefaultMaxWalkDepth)
		Current.MaxWalkDepth = DefaultMaxWalkDepth
	}

	if Current.Rate.PublicPerMin < 0 {
		Current.Rate.PublicPerMin = DefaultPublicRatePerMin
	}
	if Current.Rate.UploadPerMin < 0 {
		Current.Rate.UploadPerMin = DefaultUploadRatePerMin
	}
	if Current.Rate.Burst <= 0 {
		Current.Rate.Burst = DefaultRateBurst
	}

	if Current.Compression.Quality < 1 || Current.Compression.Quality > 100 {
		log.Printf("Warning: COMPRESSION_QUALITY %d out of range (1-100), using %d", Current.Compression.Quality, DefaultCompressionQuality)
		Current.Compression.Quality = DefaultCompressionQuality
	}
	if Current.Compression.Scale < 1 || Current.Compression.Scale > 100 {
		log.Printf("Warning: COMPRESSION_SCALE %d out of range (1-100), using %d", Current.Compression.Scale, DefaultCompressionScale)
		Current.Compression.Scale = DefaultCompressionScale
	}

	if Current.ProxyHost != "" {
		switch Current.ProxyType {
		case "http", "https", "socks5":
		default:
			log.Printf("Warning: invalid proxy type %q, using http", Current.ProxyType)
			Current.ProxyType = "http"
		}
	}

	ip, cidr, err := parseTrustedProxyValue(Current.TrustedProxy)
	if err != nil {
		log.Printf("Warning: invalid TRUSTED_PROXY %q — ignoring (must be IP or CIDR)", Current.TrustedProxy)
		Current.TrustedProxy = ""
		cachedProxyPtr.Store(&parsedProxy{})
	} else {
		cachedProxyPtr.Store(&parsedProxy{ip: ip, cidr: cidr})
	}
}
