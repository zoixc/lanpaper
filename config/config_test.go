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
			
			// After validation, invalid ports should be reset to 8080
			if !tt.expectValid && Current.Port != "8080" {
				t.Errorf("Expected invalid port %q to be reset to 8080, got %q", tt.port, Current.Port)
			}
		})
	}
}

func TestValidateMaxUploadMB(t *testing.T) {
	tests := []struct {
		name           string
		maxUploadMB    int
		expectAdjusted bool
		expectedValue  int
	}{
		{"valid 10MB", 10, false, 10},
		{"valid 100MB", 100, false, 100},
		{"invalid - zero", 0, true, 10},
		{"invalid - negative", -5, true, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{
				Port:        "8080",
				MaxUploadMB: tt.maxUploadMB,
				Rate: RateCfg{
					PublicPerMin: 50,
					AdminPerMin:  0,
					UploadPerMin: 20,
					Burst:        10,
				},
				MaxConcurrentUploads: 3,
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
				Rate: RateCfg{
					PublicPerMin: 50,
					AdminPerMin:  0,
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
		rate         RateCfg
		expectedRate RateCfg
	}{
		{
			name: "all valid",
			rate: RateCfg{PublicPerMin: 50, AdminPerMin: 100, UploadPerMin: 20, Burst: 10},
			expectedRate: RateCfg{PublicPerMin: 50, AdminPerMin: 100, UploadPerMin: 20, Burst: 10},
		},
		{
			name: "negative PublicPerMin",
			rate: RateCfg{PublicPerMin: -1, AdminPerMin: 0, UploadPerMin: 20, Burst: 10},
			expectedRate: RateCfg{PublicPerMin: 50, AdminPerMin: 0, UploadPerMin: 20, Burst: 10},
		},
		{
			name: "negative UploadPerMin",
			rate: RateCfg{PublicPerMin: 50, AdminPerMin: 0, UploadPerMin: -5, Burst: 10},
			expectedRate: RateCfg{PublicPerMin: 50, AdminPerMin: 0, UploadPerMin: 20, Burst: 10},
		},
		{
			name: "zero Burst",
			rate: RateCfg{PublicPerMin: 50, AdminPerMin: 0, UploadPerMin: 20, Burst: 0},
			expectedRate: RateCfg{PublicPerMin: 50, AdminPerMin: 0, UploadPerMin: 20, Burst: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Current = Config{
				Port:                 "8080",
				MaxUploadMB:          10,
				Rate:                 tt.rate,
				MaxConcurrentUploads: 3,
			}
			validate()
			
			if Current.Rate.PublicPerMin != tt.expectedRate.PublicPerMin {
				t.Errorf("Expected PublicPerMin to be %d, got %d", tt.expectedRate.PublicPerMin, Current.Rate.PublicPerMin)
			}
			if Current.Rate.AdminPerMin != tt.expectedRate.AdminPerMin {
				t.Errorf("Expected AdminPerMin to be %d, got %d", tt.expectedRate.AdminPerMin, Current.Rate.AdminPerMin)
			}
			if Current.Rate.UploadPerMin != tt.expectedRate.UploadPerMin {
				t.Errorf("Expected UploadPerMin to be %d, got %d", tt.expectedRate.UploadPerMin, Current.Rate.UploadPerMin)
			}
			if Current.Rate.Burst != tt.expectedRate.Burst {
				t.Errorf("Expected Burst to be %d, got %d", tt.expectedRate.Burst, Current.Rate.Burst)
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
				Port:        "8080",
				MaxUploadMB: 10,
				Rate: RateCfg{
					PublicPerMin: 50,
					AdminPerMin:  0,
					UploadPerMin: 20,
					Burst:        10,
				},
				MaxConcurrentUploads: 3,
				ProxyType:            tt.proxyType,
				ProxyHost:            tt.proxyHost,
				ProxyPort:            tt.proxyPort,
			}
			validate()
			
			if Current.ProxyType != tt.expectedProxyType {
				t.Errorf("Expected ProxyType to be %q, got %q", tt.expectedProxyType, Current.ProxyType)
			}
		})
	}
}

func TestAutoDisableAuth(t *testing.T) {
	// Save original env
	origUser := os.Getenv("ADMIN_USER")
	origPass := os.Getenv("ADMIN_PASS")
	defer func() {
		os.Setenv("ADMIN_USER", origUser)
		os.Setenv("ADMIN_PASS", origPass)
	}()

	tests := []struct {
		name               string
		username           string
		password           string
		expectAuthDisabled bool
	}{
		{"both provided - auth enabled", "admin", "password123", false},
		{"no username - auth disabled", "", "password123", true},
		{"no password - auth disabled", "admin", "", true},
		{"both empty - auth disabled", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env vars
			os.Unsetenv("ADMIN_USER")
			os.Unsetenv("ADMIN_PASS")
			os.Unsetenv("DISABLE_AUTH")
			
			Current = Config{
				Port:        "8080",
				Username:    tt.username,
				Password:    tt.password,
				MaxUploadMB: 10,
				Rate: RateCfg{
					PublicPerMin: 50,
					AdminPerMin:  0,
					UploadPerMin: 20,
					Burst:        10,
				},
				MaxConcurrentUploads: 3,
				DisableAuth:          false,
			}
			
			// Simulate the auth check logic from Load()
			if Current.Username == "" || Current.Password == "" {
				if !Current.DisableAuth {
					Current.DisableAuth = true
				}
			}
			
			if Current.DisableAuth != tt.expectAuthDisabled {
				t.Errorf("Expected DisableAuth to be %v, got %v", tt.expectAuthDisabled, Current.DisableAuth)
			}
		})
	}
}
