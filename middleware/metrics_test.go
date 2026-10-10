// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http/httptest"
	"testing"
	"time"

	"lanpaper/config"
)

func TestDiskUsageMetricIsCached(t *testing.T) {
	now := time.Now()
	diskMetricCache.Lock()
	diskMetricCache.at, diskMetricCache.bytes = now, 123
	diskMetricCache.Unlock()
	if got := diskUsageMetric(now.Add(time.Second)); got != 123 {
		t.Fatalf("cached disk usage=%d", got)
	}
}

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
