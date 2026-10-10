// SPDX-License-Identifier: MIT

package metrics

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestMetricsHaveBoundedLabelsAndRequiredSignals(t *testing.T) {
	ObserveRequest(204, 20*time.Millisecond)
	RateRejected()
	PersistenceFailure()
	done := BeginUpload()
	var out bytes.Buffer
	WritePrometheus(&out, 3, 42)
	done()
	text := out.String()
	for _, want := range []string{"status_class=\"2xx\"", "lanpaper_active_uploads 1", "lanpaper_processing_queue 0", "lanpaper_rate_limit_rejections_total", "lanpaper_active_sessions 3", "lanpaper_persistence_failures_total", "lanpaper_disk_usage_bytes 42"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "path=") || strings.Contains(text, "client=") {
		t.Fatalf("unbounded label exposed: %s", text)
	}
}
