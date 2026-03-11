package config

import (
	"crypto/rand"
	"encoding/base64"
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
	AdminPerMin  int `json:"adminPerMin"`
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
	AllowPrivateURLFetch bool              `json:"allowPrivateURLFetch,omitempty"`
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
	// CSRFSecret is the HMAC signing key for stateless CSRF tokens.
	// Set via CSRF_SECRET env var. Auto-generated at startup if not provided
	// (tokens will be invalidated on each restart in that case).
	CSRFSecret string `json:"csrfSecret,omitempty"`
}

var Current Config

// cachedProxy caches the parsed TrustedProxy value set during Load/validate.
// Stored as *parsedProxy via atomic pointer to avoid any lock on the hot path.
type parsedProxy struct {
	ip   *net.IP
	cidr *net.IPNet
}

var cachedProxyPtr atomic.Pointer[parsedProxy]

// ---------------------------------------------------------------------------
// env helpers — read once, return zero value when unset
// ---------------------------------------------------------------------------

func envStr(key string) (string, bool) {
	v := os.Getenv(key)
	return v, v != ""
}

func envInt(key string) (int, bool) {
	v := os.Getenv(key)
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

func envBool(key string) (bool, bool) {
	v := os.Getenv(key)
	if v == "" {
		return false, false
	}
	b, err := strconv.ParseBool(v)
	return b, err == nil
}

// Load loads configuration with priority: env vars > config.json > defaults.
func Load() {
	Current = Config{
		Port:                 "8080",
		MaxUploadMB:          DefaultMaxUploadMB,
		MaxConcurrentUploads: DefaultMaxConcurrentUploads,
		MaxWalkDepth:         DefaultMaxWalkDepth,
		ExternalImageDir:     "external/images",
		ProxyType:            "http",
		Rate: RateConfig{
			PublicPerMin: DefaultPublicRatePerMin,
			AdminPerMin:  DefaultAdminRatePerMin,
			UploadPerMin: DefaultUploadRatePerMin,
			Burst:        DefaultRateBurst,
		},
		Compression: CompressionConfig{
			Quality: DefaultCompressionQuality,
			Scale:   DefaultCompressionScale,
		},
	}

	// Override with config.json (if present).
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	// Override with environment variables (highest priority).
	if v, ok := envStr("PORT"); ok {
		Current.Port = v
	}
	if v, ok := envInt("MAX_UPLOAD_MB"); ok {
		Current.MaxUploadMB = v
	}
	if v, ok := envInt("MAX_IMAGES"); ok {
		Current.MaxImages = v
	}
	if v, ok := envInt("MAX_CONCURRENT_UPLOADS"); ok {
		Current.MaxConcurrentUploads = v
	}
	if v, ok := envInt("MAX_WALK_DEPTH"); ok {
		Current.MaxWalkDepth = v
	}
	if v, ok := envStr("EXTERNAL_IMAGE_DIR"); ok {
		Current.ExternalImageDir = v
	}
	if v, ok := envStr("ADMIN_USER"); ok {
		Current.AdminUser = v
	}
	if v, ok := envStr("ADMIN_PASS"); ok {
		Current.AdminPass = v
	}
	if v, ok := envBool("DISABLE_AUTH"); ok {
		Current.DisableAuth = v
	}
	if v, ok := envBool("INSECURE_SKIP_VERIFY"); ok {
		Current.InsecureSkipVerify = v
	}
	if v, ok := envBool("ALLOW_PRIVATE_URL_FETCH"); ok {
		Current.AllowPrivateURLFetch = v
	}
	if v, ok := envStr("PROXY_HOST"); ok {
		Current.ProxyHost = v
	}
	if v, ok := envStr("PROXY_PORT"); ok {
		Current.ProxyPort = v
	}
	if v, ok := envStr("PROXY_TYPE"); ok {
		Current.ProxyType = v
	}
	// Support both PROXY_USERNAME and PROXY_USER.
	if v, ok := envStr("PROXY_USERNAME"); ok {
		Current.ProxyUsername = v
	} else if v, ok := envStr("PROXY_USER"); ok {
		Current.ProxyUsername = v
	}
	// Support both PROXY_PASSWORD and PROXY_PASS.
	if v, ok := envStr("PROXY_PASSWORD"); ok {
		Current.ProxyPassword = v
	} else if v, ok := envStr("PROXY_PASS"); ok {
		Current.ProxyPassword = v
	}
	if v, ok := envStr("TRUSTED_PROXY"); ok {
		Current.TrustedProxy = v
	}
	if v, ok := envStr("CSRF_SECRET"); ok {
		Current.CSRFSecret = v
	}
	if v, ok := envInt("RATE_PUBLIC_PER_MIN"); ok {
		Current.Rate.PublicPerMin = v
	}
	if v, ok := envInt("RATE_ADMIN_PER_MIN"); ok {
		Current.Rate.AdminPerMin = v
	}
	if v, ok := envInt("RATE_UPLOAD_PER_MIN"); ok {
		Current.Rate.UploadPerMin = v
	}
	if v, ok := envInt("RATE_BURST"); ok {
		Current.Rate.Burst = v
	}
	if v, ok := envInt("COMPRESSION_QUALITY"); ok {
		Current.Compression.Quality = v
	}
	if v, ok := envInt("COMPRESSION_SCALE"); ok {
		Current.Compression.Scale = v
	}

	validate()

	mode := "compressed"
	if Current.Compression.Quality == 100 && Current.Compression.Scale == 100 {
		mode = "lossless"
	}
	log.Printf("Config loaded: compression quality=%d scale=%d (%s)",
		Current.Compression.Quality, Current.Compression.Scale, mode)
	if Current.AllowPrivateURLFetch {
		log.Printf("[SECURITY] Warning: ALLOW_PRIVATE_URL_FETCH=true — SSRF protection for private IPs is DISABLED")
	}
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

// validate normalises config values in-place, logging warnings for out-of-range
// fields and auto-correcting them to safe defaults.
func validate() {
	portStr := strings.TrimPrefix(Current.Port, ":")
	if n, err := strconv.Atoi(portStr); err != nil || n < 1 || n > 65535 {
		log.Printf("Warning: invalid port %q, using 8080", Current.Port)
		Current.Port = "8080"
	}

	if Current.MaxUploadMB < MinUploadMB {
		log.Printf("Warning: MaxUploadMB %d is below minimum %d, using %d", Current.MaxUploadMB, MinUploadMB, DefaultMaxUploadMB)
		Current.MaxUploadMB = DefaultMaxUploadMB
	}
	if Current.MaxConcurrentUploads <= 0 {
		Current.MaxConcurrentUploads = DefaultMaxConcurrentUploads
	}
	if Current.MaxWalkDepth <= 0 || Current.MaxWalkDepth > 10 {
		log.Printf("Warning: MaxWalkDepth %d out of range (1-10), using %d", Current.MaxWalkDepth, DefaultMaxWalkDepth)
		Current.MaxWalkDepth = DefaultMaxWalkDepth
	}

	if Current.Rate.PublicPerMin < 0 {
		Current.Rate.PublicPerMin = DefaultPublicRatePerMin
	}
	if Current.Rate.AdminPerMin <= 0 {
		Current.Rate.AdminPerMin = DefaultAdminRatePerMin
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

	// Auto-generate CSRF secret if not provided. Tokens will be invalidated
	// on restart in this case — set CSRF_SECRET in env for persistence.
	if Current.CSRFSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err == nil {
			Current.CSRFSecret = base64.RawURLEncoding.EncodeToString(b)
			log.Println("Warning: CSRF_SECRET not set — auto-generated ephemeral secret. " +
				"Tokens will be invalidated on restart. Set CSRF_SECRET env var for persistence.")
		}
	}

	// If auth is enabled but credentials are missing, automatically disable
	// auth rather than crashing.
	if !Current.DisableAuth && (Current.AdminUser == "" || Current.AdminPass == "") {
		log.Printf("Warning: ADMIN_USER or ADMIN_PASS is empty — authentication disabled automatically. " +
			"Set both credentials or set DISABLE_AUTH=true to suppress this warning.")
		Current.DisableAuth = true
	}
}
