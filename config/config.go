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
		ProxyUsername:        getEnv("PROXY_USERNAME", ""),
		ProxyPassword:        getEnv("PROXY_PASSWORD", ""),
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
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
