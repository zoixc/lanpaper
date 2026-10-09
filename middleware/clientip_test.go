// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"lanpaper/config"
)

// withTrustedProxy installs a trusted proxy for one test and restores the
// previous configuration afterwards.
func withTrustedProxy(t *testing.T, value string) {
	t.Helper()
	old := config.Current.TrustedProxy
	config.Current.TrustedProxy = value
	config.ApplyTrustedProxy()
	t.Cleanup(func() {
		config.Current.TrustedProxy = old
		config.ApplyTrustedProxy()
	})
}

// A trusted proxy appends the client it saw to X-Forwarded-For. A client-sent
// X-Real-IP passes straight through such a proxy, so it must never be believed.
func TestClientIPIgnoresSpoofableXRealIP(t *testing.T) {
	withTrustedProxy(t, "172.24.0.1")

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "172.24.0.1:50000"
	r.Header.Set("X-Forwarded-For", "203.0.113.7")
	r.Header.Set("X-Real-IP", "192.168.1.50")
	if got := clientIP(r); got != "203.0.113.7" {
		t.Fatalf("clientIP() = %s, want the address the proxy saw (203.0.113.7)", got)
	}
}

// Only the rightmost X-Forwarded-For entry is believed: anything to its left
// was written by the client.
func TestClientIPUsesRightmostForwardedFor(t *testing.T) {
	withTrustedProxy(t, "172.24.0.1")

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "172.24.0.1:50000"
	r.Header.Set("X-Forwarded-For", "192.168.1.50, 198.51.100.9")
	if got := clientIP(r); got != "198.51.100.9" {
		t.Fatalf("clientIP() = %s, want 198.51.100.9", got)
	}
}

// From an untrusted peer every forwarding header is ignored.
func TestClientIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	withTrustedProxy(t, "172.24.0.1")

	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	r.RemoteAddr = "203.0.113.99:40000"
	r.Header.Set("X-Forwarded-For", "192.168.1.50")
	r.Header.Set("X-Real-IP", "192.168.1.51")
	if got := clientIP(r); got != "203.0.113.99" {
		t.Fatalf("clientIP() = %s, want the socket peer 203.0.113.99", got)
	}
}
