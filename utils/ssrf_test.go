package utils

import (
	"net"
	"testing"
)

// TestPrivateRanges verifies that PrivateRanges initialises without error
// and contains entries for the expected RFC blocks.
func TestPrivateRanges(t *testing.T) {
	ranges := PrivateRanges()
	if len(ranges) == 0 {
		t.Fatal("PrivateRanges() returned empty slice")
	}
	// Spot-check: 10.0.0.1 must be covered.
	ip := net.ParseIP("10.0.0.1")
	for _, r := range ranges {
		if r.Contains(ip) {
			return
		}
	}
	t.Error("PrivateRanges() does not contain 10.0.0.0/8")
}

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		ip      string
		blocked bool
	}{
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"127.0.0.1", true},
		{"10.0.0.5", true},
		{"192.168.1.1", true},
		{"172.16.5.5", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		got := IsBlockedIP(net.ParseIP(tt.ip))
		if got != tt.blocked {
			t.Errorf("IsBlockedIP(%s) = %v, want %v", tt.ip, got, tt.blocked)
		}
	}
}

func TestIsPrivateOrLocalIP(t *testing.T) {
	tests := []struct {
		ip    string
		local bool
	}{
		{"8.8.8.8", false},
		{"127.0.0.1", true},
		{"192.168.100.100", true},
		{"10.1.2.3", true},
		{"172.18.0.1", true},
		{"100.64.1.1", true},
		{"::1", true},
	}
	for _, tt := range tests {
		got := IsPrivateOrLocalIP(net.ParseIP(tt.ip))
		if got != tt.local {
			t.Errorf("IsPrivateOrLocalIP(%s) = %v, want %v", tt.ip, got, tt.local)
		}
	}
}

func TestValidateRemoteURL(t *testing.T) {
	tests := []struct {
		raw     string
		wantErr bool
	}{
		{"http://127.0.0.1/img.png", true},
		{"http://169.254.169.254/latest/meta-data", true},
		{"http://localhost/x", true},
		{"http://10.0.0.1/x", true},
		{"ftp://example.com/x", true},
		{"not-a-url", true},
		{"https://example.com/img.png", false},
	}
	for _, tt := range tests {
		err := ValidateRemoteURL(tt.raw)
		if (err != nil) != tt.wantErr {
			t.Errorf("ValidateRemoteURL(%q) err=%v, wantErr=%v", tt.raw, err, tt.wantErr)
		}
	}
}
