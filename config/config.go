package config

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
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
		MaxImages: 100,
		ProxyType: "http",
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
}
