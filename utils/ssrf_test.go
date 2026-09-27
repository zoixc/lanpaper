package utils

import (
	"context"
	"net"
	"testing"
)

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

func TestResolvePublicURL(t *testing.T) {
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
		{"https://8.8.8.8/img.png", false},
	}
	for _, tt := range tests {
		_, err := ResolvePublicURL(context.Background(), tt.raw)
		if (err != nil) != tt.wantErr {
			t.Errorf("ResolvePublicURL(%q) err=%v, wantErr=%v", tt.raw, err, tt.wantErr)
		}
	}
}
