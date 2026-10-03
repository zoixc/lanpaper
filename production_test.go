// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The probes and robots.txt sit outside the authentication and security
// middleware, so their contract is checked here rather than in a handler test.
// Panic recovery for the same chain (Gzip → Recover → mux) is covered by
// middleware/recover_test.go.

func TestRobotsTxtKeepsCrawlersOffMedia(t *testing.T) {
	a := setupApp(t)

	status, h, body := a.request(http.MethodGet, "/robots.txt", nil, false, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /robots.txt = %d", status)
	}
	if got, want := string(body), "User-agent: *\nDisallow: /\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if ct := h.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if cc := h.Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	if h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("robots.txt is served without nosniff")
	}

	status, _, body = a.request(http.MethodHead, "/robots.txt", nil, false, nil)
	if status != http.StatusOK || len(body) != 0 {
		t.Fatalf("HEAD /robots.txt = %d with %d bytes", status, len(body))
	}

	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		if status, _, _ := a.request(method, "/robots.txt", nil, false, nil); status != http.StatusMethodNotAllowed {
			t.Fatalf("%s /robots.txt = %d, want 405", method, status)
		}
	}

	// An exact pattern must not swallow its neighbours: "robots" is not a
	// reserved link name, so /robots stays a normal (unknown) link lookup.
	if status, _, body := a.request(http.MethodGet, "/robots", nil, false, nil); status != http.StatusNotFound {
		t.Fatalf("GET /robots = %d (%s), want 404", status, body)
	}
}

func TestProbesRejectWriteMethods(t *testing.T) {
	a := setupApp(t)

	// Liveness never touches the disk, so it always answers 200.
	status, h, body := a.request(http.MethodGet, "/health", nil, false, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /health = %d", status)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("GET /health is not JSON: %v (%s)", err, body)
	}
	if payload["status"] != "ok" || payload["service"] != "lanpaper" {
		t.Fatalf("GET /health = %s", body)
	}
	if h.Get("Cache-Control") == "" || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("GET /health headers: %v", h)
	}

	for _, path := range []string{"/health", "/health/ready"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			if status, _, _ := a.request(method, path, nil, false, nil); status != http.StatusMethodNotAllowed {
				t.Fatalf("%s %s = %d, want 405", method, path, status)
			}
		}
	}
}
