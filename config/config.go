package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
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
}

var Current Config

func Load() {
	// Step 1: load config.json as base (lowest priority)
	if data, err := os.ReadFile("config.json"); err == nil {
		if err := json.Unmarshal(data, &Current); err != nil {
			log.Printf("Warning: failed to parse config.json: %v", err)
		}
	}

	// Step 2: env variables override config.json (highest priority)
	if v := os.Getenv("PORT"); v != "" {
		Current.Port = v
	}
	if Current.Port == "" {
		Current.Port = "8080"
	}

	if v := os.Getenv("MAX_UPLOAD_MB"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.MaxUploadMB = n
		}
	}
	if Current.MaxUploadMB == 0 {
		Current.MaxUploadMB = 50
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
	if Current.MaxConcurrentUploads == 0 {
		Current.MaxConcurrentUploads = 2
	}

	if v := os.Getenv("EXTERNAL_IMAGE_DIR"); v != "" {
		Current.ExternalImageDir = v
	}
	if Current.ExternalImageDir == "" {
		Current.ExternalImageDir = "external/images"
	}

	if v := os.Getenv("ADMIN_USER"); v != "" {
		Current.AdminUser = v
	}
	if v := os.Getenv("ADMIN_PASS"); v != "" {
		Current.AdminPass = v
	}

	// Booleans: only override if env is explicitly set
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
	if Current.ProxyType == "" {
		Current.ProxyType = "http"
	}
	if v := os.Getenv("PROXY_USERNAME"); v != "" {
		Current.ProxyUsername = v
	}
	if v := os.Getenv("PROXY_PASSWORD"); v != "" {
		Current.ProxyPassword = v
	}

	if v := os.Getenv("RATE_PUBLIC_PER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.Rate.PublicPerMin = n
		}
	}
	if Current.Rate.PublicPerMin == 0 {
		Current.Rate.PublicPerMin = 120
	}

	if v := os.Getenv("RATE_UPLOAD_PER_MIN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.Rate.UploadPerMin = n
		}
	}
	if Current.Rate.UploadPerMin == 0 {
		Current.Rate.UploadPerMin = 20
	}

	if v := os.Getenv("RATE_BURST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			Current.Rate.Burst = n
		}
	}
	if Current.Rate.Burst == 0 {
		Current.Rate.Burst = 10
	}
}
