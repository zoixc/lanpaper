// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http/httptest"
	"testing"

	"lanpaper/config"
)

func TestDisabledMetricsDoesNotAdvertiseAuthentication(t *testing.T) {
	old := config.Current
	t.Cleanup(func() { config.Current = old })
	config.Current.MetricsEnabled = false
	req := httptest.NewRequest("GET", "http://example.test/metrics", nil)
	rec := httptest.NewRecorder()
	MetricsEndpoint(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != "" {
		t.Fatalf("disabled endpoint advertised auth: %q", got)
	}
}
