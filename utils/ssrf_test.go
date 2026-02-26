package utils

import (
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	tests := []struct {
		host    string
		blocked bool
	}{
		// Must be blocked
		{"127.0.0.1", true},
		{"localhost", true},
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true}, // AWS metadata
		{"100.64.0.1", true},      // CGNAT
		{"::1", true},             // IPv6 loopback
		{"this.host.does.not.exist.invalid", true}, // unresolvable -> fail safe
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := IsBlockedIP(tt.host)
			if got != tt.blocked {
				t.Errorf("IsBlockedIP(%q) = %v, want %v", tt.host, got, tt.blocked)
			}
		})
	}
}
