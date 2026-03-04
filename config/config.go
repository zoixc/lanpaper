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
// This ensures environment variables always override config.json settings.
func Load() {
	// Step 1: Load defaults
	Current = Config{
		Port:                 "8080",
		MaxUploadMB:          DefaultMaxUploadMB,
		MaxImages:            0,
		MaxConcurrentUploads: DefaultMaxConcurrentUploads,
		MaxWalkDepth:         DefaultMaxWalkDepth,
		ExternalImageDir:     "external/images",
		AdminUser:            "",
		AdminPass:            "",
		DisableAuth:          false,
		InsecureSkipVerify:   false,
		ProxyHost:            "",
		ProxyPort:            "",
		ProxyType:            "http",
		ProxyUsername:        "",
		ProxyPassword:        "",
		TrustedProxy:         "",
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
	log.Printf("Config: loaded defaults (compression: quality=%d, scale=%d)", Current.Compression.Quality, Current.Compression.Scale)

	// Step 2: Override with config.json (if exists)
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		} else {
			log.Printf("Config: loaded config.json (compression: quality=%d, scale=%d)", Current.Compression.Quality, Current.Compression.Scale)
		}
	} else {
		log.Printf("Config: config.json not found, using defaults")
	}

	// Step 3: Override with environment variables (highest priority)
	if v := os.Getenv("PORT"); v != "" {
		Current.Port = v
	}
	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.MaxUploadMB = n
		}
	}
	if v := os.Getenv("MAX_IMAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.MaxImages = n
		}
	}
	if v := os.Getenv("MAX_CONCURRENT_UPLOADS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.MaxConcurrentUploads = n
		}
	}
	if v := os.Getenv("MAX_WALK_DEPTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.MaxWalkDepth = n
		}
	}
	if v := os.Getenv("EXTERNAL_IMAGE_DIR"); v != "" {
		Current.ExternalImageDir = v
	}
	if v := os.Getenv("ADMIN_USER"); v != "" {
		Current.AdminUser = v
	}
	if v := os.Getenv("ADMIN_PASS"); v != "" {
		Current.AdminPass = v
	}
	if v := os.Getenv("DISABLE_AUTH"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			Current.DisableAuth = b
		}
	}
	if v := os.Getenv("INSECURE_SKIP_VERIFY"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			Current.InsecureSkipVerify = b
		}
	}
	if v := os.Getenv("PROXY_HOST"); v != "" {
		Current.ProxyHost = v
	}
	if v := os.Getenv("PROXY_PORT"); v != "" {
		Current.ProxyPort = v
	}
	if v := os.Getenv("PROXY_TYPE"); v != "" {
		Current.ProxyType = v
	}
	// Support both PROXY_USERNAME and PROXY_USER
	if v := os.Getenv("PROXY_USERNAME"); v != "" {
		Current.ProxyUsername = v
	} else if v := os.Getenv("PROXY_USER"); v != "" {
		Current.ProxyUsername = v
	}
	// Support both PROXY_PASSWORD and PROXY_PASS
	if v := os.Getenv("PROXY_PASSWORD"); v != "" {
		Current.ProxyPassword = v
	} else if v := os.Getenv("PROXY_PASS"); v != "" {
		Current.ProxyPassword = v
	}
	if v := os.Getenv("TRUSTED_PROXY"); v != "" {
		Current.TrustedProxy = v
	}

	// Rate limiting overrides
	if v := os.Getenv("RATE_PUBLIC_PER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.Rate.PublicPerMin = n
		}
	}
	if v := os.Getenv("RATE_UPLOAD_PER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.Rate.UploadPerMin = n
		}
	}
	if v := os.Getenv("RATE_BURST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.Rate.Burst = n
		}
	}

	// Compression overrides (highest priority)
	envQuality := os.Getenv("COMPRESSION_QUALITY")
	envScale := os.Getenv("COMPRESSION_SCALE")
	if envQuality != "" {
		if n, err := strconv.Atoi(envQuality); err == nil {
			log.Printf("Config: COMPRESSION_QUALITY env override: %d -> %d", Current.Compression.Quality, n)
			Current.Compression.Quality = n
		}
	}
	if envScale != "" {
		if n, err := strconv.Atoi(envScale); err == nil {
			log.Printf("Config: COMPRESSION_SCALE env override: %d -> %d", Current.Compression.Scale, n)
			Current.Compression.Scale = n
		}
	}

	validate()
	
	// Final compression config log
	mode := "compressed"
	if Current.Compression.Quality == 100 && Current.Compression.Scale == 100 {
		mode = "LOSSLESS"
	}
	log.Printf("Config: final compression settings - quality=%d, scale=%d (%s mode)", 
		Current.Compression.Quality, Current.Compression.Scale, mode)
}

// parseTrustedProxyValue parses a single TrustedProxy string.
// Returns (nil, nil, nil) when empty, and (nil, nil, err) on bad input.
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
// Uses a cached parsed value so no allocation occurs on the hot path.
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

	// Parse and cache TrustedProxy once so IsTrustedProxy is allocation-free per request.
	ip, cidr, err := parseTrustedProxyValue(Current.TrustedProxy)
	if err != nil {
		log.Printf("Warning: invalid TRUSTED_PROXY %q — ignoring (must be IP or CIDR)", Current.TrustedProxy)
		Current.TrustedProxy = ""
		cachedProxyPtr.Store(&parsedProxy{})
	} else {
		cachedProxyPtr.Store(&parsedProxy{ip: ip, cidr: cidr})
	}

	if !Current.DisableAuth && (Current.AdminUser == "" || Current.AdminPass == "") {
		Current.DisableAuth = true
	}
}
