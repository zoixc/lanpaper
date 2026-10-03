// SPDX-License-Identifier: MIT

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
		{"::ffff:127.0.0.1", true},      // IPv4-mapped loopback
		{"::ffff:8.8.8.8", false},       // IPv4-mapped public
		{"::127.0.0.1", true},           // deprecated IPv4-compatible
		{"64:ff9b::a00:1", true},        // NAT64 of 10.0.0.1
		{"2001:0:4136:e378::1", true},   // Teredo
		{"2002:c0a8:101::1", true},      // 6to4 of 192.168.1.1
		{"100::1", true},                // discard-only
		{"fec0::1", true},               // site-local
		{"fd00::1", true},               // unique local
		{"192.88.99.1", true},           // 6to4 relay anycast
		{"255.255.255.255", true},       // broadcast
		{"0.0.0.0", true},               // unspecified
		{"2606:4700:4700::1111", false}, // public IPv6
		{"198.51.100.20", true},         // documentation
		{"100.127.255.254", true},       // CGNAT upper bound
		{"100.128.0.1", false},          // just outside CGNAT
	}
	if !IsBlockedIP(nil) {
		t.Error("nil IP must be blocked")
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
		{"fd12:3456::1", true},
		{"fe80::1", true},
		{"169.254.10.1", true},
		{"::ffff:192.168.1.5", true},
		{"2001:4860:4860::8888", false},
		{"100.128.0.1", false},
	}
	if IsPrivateOrLocalIP(nil) {
		t.Error("nil IP must not count as local")
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
