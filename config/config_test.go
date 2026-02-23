package config

import (
	"os"
	"testing"
)

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name        string
		port        string
		expectValid bool
	}{
		{"valid port", "8080", true},
		{"valid port with colon", ":8080", true},
		{"valid high port", "65535", true},
		{"invalid - too high", "65536", false},
		{"invalid - zero", "0", false},
		{"invalid - negative", "-1", false},
		{"invalid - non-numeric", "abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{Port: tt.port}
			validate()

			if !tt.expectValid && Current.Port != "8080" {
				t.Errorf("Expected invalid port %q to be reset to 8080, got %q", tt.port, Current.Port)
			}
		})
	}
}

func TestValidateMaxUploadMB(t *testing.T) {
	tests := []struct {
		name          string
		maxUploadMB   int
		expectedValue int
	}{
		{"valid 10MB", 10, 10},
		{"valid 100MB", 100, 100},
		{"invalid - zero", 0, 10},
		{"invalid - negative", -5, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{
				Port:                 "8080",
				MaxUploadMB:          tt.maxUploadMB,
				MaxConcurrentUploads: 3,
				AdminUser:            "admin",
				AdminPass:            "pass",
				Rate: RateConfig{
					PublicPerMin: 50,
					UploadPerMin: 20,
					Burst:        10,
				},
			}
			validate()

			if Current.MaxUploadMB != tt.expectedValue {
				t.Errorf("Expected MaxUploadMB to be %d, got %d", tt.expectedValue, Current.MaxUploadMB)
			}
		})
	}
}

func TestValidateMaxConcurrentUploads(t *testing.T) {
	tests := []struct {
		name          string
		maxConcurrent int
		expectedValue int
	}{
		{"valid 3", 3, 3},
		{"valid 10", 10, 10},
		{"invalid - zero", 0, 2},
		{"invalid - negative", -1, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{
				Port:                 "8080",
				MaxUploadMB:          10,
				MaxConcurrentUploads: tt.maxConcurrent,
				AdminUser:            "admin",
				AdminPass:            "pass",
				Rate: RateConfig{
					PublicPerMin: 50,
					UploadPerMin: 20,
					Burst:        10,
				},
			}
			validate()

			if Current.MaxConcurrentUploads != tt.expectedValue {
				t.Errorf("Expected MaxConcurrentUploads to be %d, got %d", tt.expectedValue, Current.MaxConcurrentUploads)
			}
		})
	}
}

func TestValidateRateLimits(t *testing.T) {
	tests := []struct {
		name         string
		rate         RateConfig
		expectedRate RateConfig
	}{
		{
			name:         "all valid",
			rate:         RateConfig{PublicPerMin: 50, UploadPerMin: 20, Burst: 10},
			expectedRate: RateConfig{PublicPerMin: 50, UploadPerMin: 20, Burst: 10},
		},
		{
			name:         "negative PublicPerMin",
			rate:         RateConfig{PublicPerMin: -1, UploadPerMin: 20, Burst: 10},
			expectedRate: RateConfig{PublicPerMin: 120, UploadPerMin: 20, Burst: 10},
		},
		{
			name:         "negative UploadPerMin",
			rate:         RateConfig{PublicPerMin: 50, UploadPerMin: -5, Burst: 10},
			expectedRate: RateConfig{PublicPerMin: 50, UploadPerMin: 20, Burst: 10},
		},
		{
			name:         "zero Burst",
			rate:         RateConfig{PublicPerMin: 50, UploadPerMin: 20, Burst: 0},
			expectedRate: RateConfig{PublicPerMin: 50, UploadPerMin: 20, Burst: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{
				Port:                 "8080",
				MaxUploadMB:          10,
				MaxConcurrentUploads: 3,
				AdminUser:            "admin",
				AdminPass:            "pass",
				Rate:                 tt.rate,
			}
			validate()

			if Current.Rate.PublicPerMin != tt.expectedRate.PublicPerMin {
				t.Errorf("PublicPerMin: expected %d, got %d", tt.expectedRate.PublicPerMin, Current.Rate.PublicPerMin)
			}
			if Current.Rate.UploadPerMin != tt.expectedRate.UploadPerMin {
				t.Errorf("UploadPerMin: expected %d, got %d", tt.expectedRate.UploadPerMin, Current.Rate.UploadPerMin)
			}
			if Current.Rate.Burst != tt.expectedRate.Burst {
				t.Errorf("Burst: expected %d, got %d", tt.expectedRate.Burst, Current.Rate.Burst)
			}
		})
	}
}

func TestValidateProxyConfig(t *testing.T) {
	tests := []struct {
		name              string
		proxyType         string
		proxyHost         string
		proxyPort         string
		expectedProxyType string
	}{
		{"valid http proxy", "http", "proxy.example.com", "8080", "http"},
		{"valid https proxy", "https", "proxy.example.com", "8080", "https"},
		{"valid socks5 proxy", "socks5", "proxy.example.com", "1080", "socks5"},
		{"invalid proxy type", "ftp", "proxy.example.com", "8080", "http"},
		{"no proxy configured", "", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{
				Port:                 "8080",
				MaxUploadMB:          10,
				MaxConcurrentUploads: 3,
				AdminUser:            "admin",
				AdminPass:            "pass",
				Rate: RateConfig{
					PublicPerMin: 50,
					UploadPerMin: 20,
					Burst:        10,
				},
				ProxyType: tt.proxyType,
				ProxyHost: tt.proxyHost,
				ProxyPort: tt.proxyPort,
			}
			validate()

			if Current.ProxyType != tt.expectedProxyType {
				t.Errorf("Expected ProxyType to be %q, got %q", tt.expectedProxyType, Current.ProxyType)
			}
		})
	}
}

func TestAutoDisableAuth(t *testing.T) {
	origUser := os.Getenv("ADMIN_USER")
	origPass := os.Getenv("ADMIN_PASS")
	defer func() {
		os.Setenv("ADMIN_USER", origUser)
		os.Setenv("ADMIN_PASS", origPass)
	}()

	tests := []struct {
		name               string
		adminUser          string
		adminPass          string
		expectAuthDisabled bool
	}{
		{"both provided - auth enabled", "admin", "password123", false},
		{"no username - auth disabled", "", "password123", true},
		{"no password - auth disabled", "admin", "", true},
		{"both empty - auth disabled", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Unsetenv("ADMIN_USER")
			os.Unsetenv("ADMIN_PASS")
			os.Unsetenv("DISABLE_AUTH")

			Current = Config{
				Port:                 "8080",
				AdminUser:            tt.adminUser,
				AdminPass:            tt.adminPass,
				MaxUploadMB:          10,
				MaxConcurrentUploads: 3,
				DisableAuth:          false,
				Rate: RateConfig{
					PublicPerMin: 50,
					UploadPerMin: 20,
					Burst:        10,
				},
			}
			validate()

			if Current.DisableAuth != tt.expectAuthDisabled {
				t.Errorf("Expected DisableAuth=%v, got %v", tt.expectAuthDisabled, Current.DisableAuth)
			}
		})
	}
}
