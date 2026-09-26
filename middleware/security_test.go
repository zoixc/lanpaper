package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSameOriginRequest(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		host    string
		headers map[string]string
		want    bool
	}{
		{
			name:   "no headers (non-browser client)",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			want:   true,
		},
		{
			name:   "same-origin Origin header",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Origin": "http://lanpaper.local:8080",
			},
			want: true,
		},
		{
			name:   "same-origin Origin without port",
			method: http.MethodPost,
			host:   "lanpaper.local",
			headers: map[string]string{
				"Origin": "https://lanpaper.local",
			},
			want: true,
		},
		{
			name:   "cross-origin Origin header",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Origin": "https://evil.example",
			},
			want: false,
		},
		{
			name:   "sibling subdomain Origin header",
			method: http.MethodPost,
			host:   "pics.duckdns.org",
			headers: map[string]string{
				"Origin": "http://evil.duckdns.org",
			},
			want: false,
		},
		{
			name:   "Sec-Fetch-Site same-origin",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Sec-Fetch-Site": "same-origin",
				"Origin":         "http://lanpaper.local:8080",
			},
			want: true,
		},
		{
			name:   "Sec-Fetch-Site cross-site",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
				"Origin":         "http://lanpaper.local:8080", // spoofed
			},
			want: false,
		},
		{
			name:   "Sec-Fetch-Site cross-site without Origin",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Sec-Fetch-Site": "cross-site",
			},
			want: false,
		},
		{
			name:   "Sec-Fetch-Site none (user navigation)",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Sec-Fetch-Site": "none",
			},
			want: true,
		},
		{
			name:   "X-Forwarded-Host ignored without trusted proxy (CSRF defence)",
			method: http.MethodPost,
			host:   "lanpaper:8080",
			headers: map[string]string{
				"Origin":           "https://walls.example.com",
				"X-Forwarded-Host": "walls.example.com",
			},
			// Without TrustedProxy the XFH header must be ignored, so Origin
			// does not match Host and the request is rejected.
			want: false,
		},
		{
			name:   "Origin not in X-Forwarded-Host chain",
			method: http.MethodPost,
			host:   "lanpaper:8080",
			headers: map[string]string{
				"Origin":           "https://evil.example",
				"X-Forwarded-Host": "walls.example.com",
			},
			want: false,
		},
		{
			name:   "spoofed X-Forwarded-Host must not grant CSRF bypass",
			method: http.MethodPost,
			host:   "victim.example",
			headers: map[string]string{
				"Origin":           "https://evil.example",
				"X-Forwarded-Host": "evil.example",
			},
			want: false,
		},
		{
			name:   "malformed Origin",
			method: http.MethodPost,
			host:   "lanpaper.local:8080",
			headers: map[string]string{
				"Origin": "http://[::1]:namedport",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, "http://"+tt.host+"/api/link", nil)
			r.Host = tt.host
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if got := sameOriginRequest(r); got != tt.want {
				t.Errorf("sameOriginRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithSecurityBlocksCrossSitePost(t *testing.T) {
	called := false
	handler := WithSecurity(func(w http.ResponseWriter, r *http.Request) { called = true })

	r := httptest.NewRequest(http.MethodPost, "http://lanpaper.local:8080/api/link", nil)
	r.Host = "lanpaper.local:8080"
	r.Header.Set("Origin", "https://evil.example")
	w := httptest.NewRecorder()
	handler(w, r)

	if called {
		t.Error("handler was called for a cross-site POST")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWithSecurityAllowsSameOriginPost(t *testing.T) {
	called := false
	handler := WithSecurity(func(w http.ResponseWriter, r *http.Request) { called = true })

	r := httptest.NewRequest(http.MethodPost, "http://lanpaper.local:8080/api/link", nil)
	r.Header.Set("Origin", "http://lanpaper.local:8080")
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Error("handler was not called for a same-origin POST")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if v := w.Header().Get("Content-Security-Policy"); v == "" {
		t.Error("CSP header missing")
	}
	if v := w.Header().Get("X-Content-Type-Options"); v != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", v)
	}
}

func TestWithSecurityNeverBlocksGet(t *testing.T) {
	called := false
	handler := WithSecurity(func(w http.ResponseWriter, r *http.Request) { called = true })

	r := httptest.NewRequest(http.MethodGet, "http://lanpaper.local:8080/api/wallpapers", nil)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	handler(w, r)

	if !called {
		t.Error("GET request with cross-site headers was blocked")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
