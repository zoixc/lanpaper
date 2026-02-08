package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"
)

type RateCfg struct {
	PublicPerMin int `json:"public_per_min"`
	AdminPerMin  int `json:"admin_per_min"`
	UploadPerMin int `json:"upload_per_min"`
	Burst        int `json:"burst"`
}

type Config struct {
	Port                 string  `json:"port"`
	Username             string  `json:"username"`
	Password             string  `json:"password"`
	MaxUploadMB          int     `json:"maxUploadMB"`
	Rate                 RateCfg `json:"rate"`
	MaxConcurrentUploads int     `json:"max_concurrent_uploads"`

	// Proxy
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
	ProxyType          string `json:"proxyType,omitempty"`
	ProxyHost          string `json:"proxyHost,omitempty"`
	ProxyPort          string `json:"proxyPort,omitempty"`
	ProxyUsername      string `json:"proxyUsername,omitempty"`
	ProxyPassword      string `json:"proxyPassword,omitempty"`

	// Auth
	DisableAuth bool `json:"disableAuth,omitempty"`

	// Features
	MaxImages int `json:"maxImages,omitempty"`

	// External
	ExternalImageDir string `json:"externalImageDir,omitempty"`
}

var Current Config

func Load() {
	Current = Config{
		Port:        "8080",
		MaxUploadMB: 10,
		Rate: RateCfg{
			PublicPerMin: 50,
			AdminPerMin:  0,
			UploadPerMin: 20,
			Burst:        10,
		},
		MaxImages:            100,
		ProxyType:            "http",
		MaxConcurrentUploads: 3,
	}

	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	override := func(target *string, keys ...string) {
		for _, k := range keys {
			if v := os.Getenv(k); v != "" {
				*target = v
				return
			}
		}
	}

	override(&Current.Port, "PORT")
	override(&Current.Username, "ADMIN_USER", "USERNAME")
	override(&Current.Password, "ADMIN_PASS", "PASSWORD")
	override(&Current.ProxyType, "PROXY_TYPE")
	override(&Current.ProxyHost, "PROXY_HOST", "PROXY_ADDRESS")
	override(&Current.ProxyPort, "PROXY_PORT")
	override(&Current.ProxyUsername, "PROXY_USER", "PROXY_USERNAME", "PROXY_LOGIN")
	override(&Current.ProxyPassword, "PROXY_PASS", "PROXY_PASSWORD")
	override(&Current.ExternalImageDir, "EXTERNAL_IMAGE_DIR", "IMAGE_FOLDER")

	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			Current.MaxUploadMB = n
		}
	}
	if v := os.Getenv("INSECURE_SKIP_VERIFY"); v == "true" {
		Current.InsecureSkipVerify = true
	}
	if v := os.Getenv("RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			Current.Rate.PublicPerMin = n
			Current.Rate.AdminPerMin = n
			Current.Rate.UploadPerMin = n
		}
	}
	if v := os.Getenv("DISABLE_AUTH"); v == "true" {
		Current.DisableAuth = true
	}
	if v := os.Getenv("MAX_IMAGES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			Current.MaxImages = n
		}
	}

	// Validate configuration
	validate()

	// Auto-disable auth if no credentials provided
	if Current.Username == "" || Current.Password == "" {
		if !Current.DisableAuth {
			log.Println("Warning: No credentials provided (ADMIN_USER/ADMIN_PASS or username/password in config.json). Authentication is automatically disabled.")
			Current.DisableAuth = true
		}
	}

	// Security warnings
	if Current.InsecureSkipVerify {
		log.Println("⚠️  WARNING: TLS certificate verification is disabled (InsecureSkipVerify=true). This is insecure for production!")
	}

	if !Current.DisableAuth && len(Current.Password) < 16 {
		log.Println("⚠️  WARNING: Password is shorter than 16 characters. Use a stronger password for production.")
	}
}

func validate() {
	// Validate port
	port := strings.TrimPrefix(Current.Port, ":")
	if portNum, err := strconv.Atoi(port); err != nil || portNum < 1 || portNum > 65535 {
		log.Printf("Warning: Invalid port '%s', using default 8080", Current.Port)
		Current.Port = "8080"
	}

	// Validate MaxUploadMB
	if Current.MaxUploadMB < 1 {
		log.Printf("Warning: MaxUploadMB must be at least 1, setting to 10")
		Current.MaxUploadMB = 10
	}
	if Current.MaxUploadMB > 500 {
		log.Printf("Warning: MaxUploadMB is very high (%d MB), this may cause memory issues", Current.MaxUploadMB)
	}

	// Validate MaxConcurrentUploads
	if Current.MaxConcurrentUploads < 1 {
		log.Printf("Warning: MaxConcurrentUploads must be at least 1, setting to 2")
		Current.MaxConcurrentUploads = 2
	}
	if Current.MaxConcurrentUploads > 50 {
		log.Printf("Warning: MaxConcurrentUploads is very high (%d), this may cause resource exhaustion", Current.MaxConcurrentUploads)
	}

	// Validate rate limits
	if Current.Rate.PublicPerMin < 0 {
		log.Printf("Warning: PublicPerMin cannot be negative, setting to 50")
		Current.Rate.PublicPerMin = 50
	}
	if Current.Rate.AdminPerMin < 0 {
		log.Printf("Warning: AdminPerMin cannot be negative, setting to 0 (unlimited)")
		Current.Rate.AdminPerMin = 0
	}
	if Current.Rate.UploadPerMin < 0 {
		log.Printf("Warning: UploadPerMin cannot be negative, setting to 20")
		Current.Rate.UploadPerMin = 20
	}
	if Current.Rate.Burst < 1 {
		log.Printf("Warning: Burst must be at least 1, setting to 10")
		Current.Rate.Burst = 10
	}

	// Validate MaxImages
	if Current.MaxImages < 0 {
		log.Printf("Warning: MaxImages cannot be negative, setting to 100")
		Current.MaxImages = 100
	}

	// Validate proxy configuration
	if Current.ProxyHost != "" {
		if Current.ProxyType != "http" && Current.ProxyType != "https" && Current.ProxyType != "socks5" {
			log.Printf("Warning: Invalid ProxyType '%s', must be http, https, or socks5. Using http.", Current.ProxyType)
			Current.ProxyType = "http"
		}
		if Current.ProxyPort == "" {
			log.Println("Warning: ProxyHost is set but ProxyPort is empty")
		}
	}
}
