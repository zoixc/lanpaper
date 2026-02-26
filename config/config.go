package config

import (
	"encoding/json"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
)

type RateConfig struct {
	PublicPerMin int `json:"publicPerMin"`
	UploadPerMin int `json:"uploadPerMin"`
	Burst        int `json:"burst"`
}

type CompressionConfig struct {
	Quality int `json:"quality"` // 1-100, JPEG quality
	Scale   int `json:"scale"`   // 1-100, percentage of max dimensions (1920x1080)
}

type Config struct {
	Port                 string            `json:"port"`
	MaxUploadMB          int               `json:"maxUploadMB"`
	MaxImages            int               `json:"maxImages"`
	MaxConcurrentUploads int               `json:"maxConcurrentUploads"`
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
	// Leave empty to always use the raw TCP remote address (safe default).
	TrustedProxy string `json:"trustedProxy,omitempty"`
}

var Current Config

func Load() {
	Current = Config{
		Port:                 getEnv("PORT", "8080"),
		MaxUploadMB:          getEnvInt("MAX_UPLOAD_MB", DefaultMaxUploadMB),
		MaxImages:            getEnvInt("MAX_IMAGES", 0),
		MaxConcurrentUploads: getEnvInt("MAX_CONCURRENT_UPLOADS", DefaultMaxConcurrentUploads),
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

// IsTrustedProxy reports whether remoteAddr matches the configured TrustedProxy
// IP or CIDR. Returns false when TrustedProxy is empty.
func IsTrustedProxy(remoteAddr string) bool {
	if Current.TrustedProxy == "" {
		return false
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if proxyIP := net.ParseIP(Current.TrustedProxy); proxyIP != nil {
		return proxyIP.Equal(ip)
	}
	_, cidr, err := net.ParseCIDR(Current.TrustedProxy)
	if err != nil {
		return false
	}
	return cidr.Contains(ip)
}

// validate sanitises Current in-place, resetting out-of-range values to safe
// defaults. Called by Load and available to tests.
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

	if Current.Rate.PublicPerMin < 0 {
		Current.Rate.PublicPerMin = DefaultPublicRatePerMin
	}
	if Current.Rate.UploadPerMin < 0 {
		Current.Rate.UploadPerMin = DefaultUploadRatePerMin
	}
	if Current.Rate.Burst <= 0 {
		Current.Rate.Burst = DefaultRateBurst
	}

	// Validate compression settings
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

	if Current.TrustedProxy != "" {
		valid := net.ParseIP(Current.TrustedProxy) != nil
		if !valid {
			_, _, err := net.ParseCIDR(Current.TrustedProxy)
			valid = err == nil
		}
		if !valid {
			log.Printf("Warning: invalid TRUSTED_PROXY %q — ignoring (must be IP or CIDR)", Current.TrustedProxy)
			Current.TrustedProxy = ""
		}
	}

	// Both username and password are required; missing either disables auth.
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

// getEnvAny returns the first non-empty value among the given env keys.
// The last argument is the fallback.
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
