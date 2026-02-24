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

type Config struct {
	Port                 string     `json:"port"`
	MaxUploadMB          int        `json:"maxUploadMB"`
	MaxImages            int        `json:"maxImages"`
	MaxConcurrentUploads int        `json:"maxConcurrentUploads"`
	ExternalImageDir     string     `json:"externalImageDir"`
	AdminUser            string     `json:"adminUser"`
	AdminPass            string     `json:"adminPass"`
	DisableAuth          bool       `json:"disableAuth,omitempty"`
	InsecureSkipVerify   bool       `json:"insecureSkipVerify,omitempty"`
	ProxyHost            string     `json:"proxyHost,omitempty"`
	ProxyPort            string     `json:"proxyPort,omitempty"`
	ProxyType            string     `json:"proxyType,omitempty"`
	ProxyUsername        string     `json:"proxyUsername,omitempty"`
	ProxyPassword        string     `json:"proxyPassword,omitempty"`
	Rate                 RateConfig `json:"rate"`
	// TrustedProxy is the IP (or CIDR) of a reverse proxy sitting in front of
	// Lanpaper. When set, X-Real-IP / X-Forwarded-For headers are trusted ONLY
	// for requests that arrive from this address. Leave empty to use the
	// direct TCP remote address (safe default for LAN / direct deployments).
	TrustedProxy string `json:"trustedProxy,omitempty"`
}

var Current Config

func Load() {
	Current = Config{
		Port:                 getEnv("PORT", "8080"),
		MaxUploadMB:          getEnvInt("MAX_UPLOAD_MB", 50),
		MaxImages:            getEnvInt("MAX_IMAGES", 0),
		MaxConcurrentUploads: getEnvInt("MAX_CONCURRENT_UPLOADS", 2),
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
			PublicPerMin: getEnvInt("RATE_PUBLIC_PER_MIN", 120),
			UploadPerMin: getEnvInt("RATE_UPLOAD_PER_MIN", 20),
			Burst:        getEnvInt("RATE_BURST", 10),
		},
	}

	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	validate()
}

// IsTrustedProxy reports whether the given remote address matches the
// configured TrustedProxy IP or CIDR. Always returns false when TrustedProxy
// is empty (safe default).
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
	// Accept exact IP match
	if proxyIP := net.ParseIP(Current.TrustedProxy); proxyIP != nil {
		return proxyIP.Equal(ip)
	}
	// Accept CIDR match
	_, cidr, err := net.ParseCIDR(Current.TrustedProxy)
	if err != nil {
		return false
	}
	return cidr.Contains(ip)
}

// validate sanitises Current in-place, resetting any out-of-range values to
// safe defaults. It is called by Load() and is also available to tests.
func validate() {
	// Port
	portStr := strings.TrimPrefix(Current.Port, ":")
	if n, err := strconv.Atoi(portStr); err != nil || n < 1 || n > 65535 {
		log.Printf("Warning: invalid port %q, using 8080", Current.Port)
		Current.Port = "8080"
	}

	// MaxUploadMB
	if Current.MaxUploadMB <= 0 {
		log.Printf("Warning: invalid MaxUploadMB %d, using 10", Current.MaxUploadMB)
		Current.MaxUploadMB = 10
	}

	// MaxConcurrentUploads
	if Current.MaxConcurrentUploads <= 0 {
		Current.MaxConcurrentUploads = 2
	}

	// Rate limits
	if Current.Rate.PublicPerMin < 0 {
		Current.Rate.PublicPerMin = 120
	}
	if Current.Rate.UploadPerMin < 0 {
		Current.Rate.UploadPerMin = 20
	}
	if Current.Rate.Burst <= 0 {
		Current.Rate.Burst = 10
	}

	// Proxy type
	if Current.ProxyHost != "" {
		switch Current.ProxyType {
		case "http", "https", "socks5":
			// valid
		default:
			log.Printf("Warning: invalid proxy type %q, using http", Current.ProxyType)
			Current.ProxyType = "http"
		}
	}

	// Validate TrustedProxy format if set
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

	// Auto-disable auth when either credential is missing.
	// Both username AND password must be provided for auth to work.
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
// The last argument is the fallback value.
func getEnvAny(keys ...string) string {
	fallback := keys[len(keys)-1]
	for _, key := range keys[:len(keys)-1] {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return fallback
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
