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

func Load() {
	Current = Config{
		Port:                 getEnv("PORT", "8080"),
		MaxUploadMB:          getEnvInt("MAX_UPLOAD_MB", DefaultMaxUploadMB),
		MaxImages:            getEnvInt("MAX_IMAGES", 0),
		MaxConcurrentUploads: getEnvInt("MAX_CONCURRENT_UPLOADS", DefaultMaxConcurrentUploads),
		MaxWalkDepth:         getEnvInt("MAX_WALK_DEPTH", DefaultMaxWalkDepth),
		ExternalImageDir:     getEnv("EXTERNAL_IMAGE_DIR", "external/images"),
		AdminUser:            getEnv("ADMIN_USER", ""),
		AdminPass:            getEnv("ADMIN_PASS", ""),
		DisableAuth:          getEnvBool("DISABLE_AUTH", false),
		InsecureSkipVerify:   getEnvBool("INSECURE_SKIP_VERIFY", false),
		ProxyHost:            getEnv("PROXY_HOST", ""),
		ProxyPort:            getEnv("PROXY_PORT", ""),
		ProxyType:            getEnv("PROXY_TYPE", "http"),
		ProxyUsername:        getEnvAny("PROXY_USERNAME", "PROXY_USER", ""),
		ProxyPassword:        getEnvAny("PROXY_PASSWORD", "PROXY_PASS", ""),
		TrustedProxy:         getEnv("TRUSTED_PROXY", ""),
		Rate: RateConfig{
			PublicPerMin: getEnvInt("RATE_PUBLIC_PER_MIN", DefaultPublicRatePerMin),
			UploadPerMin: getEnvInt("RATE_UPLOAD_PER_MIN", DefaultUploadRatePerMin),
			Burst:        getEnvInt("RATE_BURST", DefaultRateBurst),
		},
		Compression: CompressionConfig{
			Quality: getEnvInt("COMPRESSION_QUALITY", DefaultCompressionQuality),
			Scale:   getEnvInt("COMPRESSION_SCALE", DefaultCompressionScale),
		},
	}

	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	validate()
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

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvAny returns the first non-empty value among the given env keys;
// the last argument is the fallback.
func getEnvAny(keys ...string) string {
	for _, key := range keys[:len(keys)-1] {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return keys[len(keys)-1]
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
