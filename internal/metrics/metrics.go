// SPDX-License-Identifier: MIT

package metrics

import (
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

var requests [5]atomic.Uint64
var latency [6]atomic.Uint64
var activeUploads atomic.Int64
var rateRejected atomic.Uint64
var persistenceFailures atomic.Uint64
var buckets = []time.Duration{10 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, 500 * time.Millisecond, time.Second}

func ObserveRequest(status int, d time.Duration) {
	class := status / 100
	if class < 1 || class > 5 {
		class = 5
	}
	requests[class-1].Add(1)
	placed := false
	for i, b := range buckets {
		if d <= b {
			latency[i].Add(1)
			placed = true
			break
		}
	}
	if !placed {
		latency[len(latency)-1].Add(1)
	}
}
func BeginUpload() func() { activeUploads.Add(1); return func() { activeUploads.Add(-1) } }
func RateRejected()       { rateRejected.Add(1) }
func PersistenceFailure() { persistenceFailures.Add(1) }
func WritePrometheus(w io.Writer, sessionCount int, diskBytes int64) {
	fmt.Fprintln(w, "# HELP lanpaper_http_requests_total HTTP requests by status class.")
	fmt.Fprintln(w, "# TYPE lanpaper_http_requests_total counter")
	for i := range requests {
		fmt.Fprintf(w, "lanpaper_http_requests_total{status_class=\"%dxx\"} %d\n", i+1, requests[i].Load())
	}
	fmt.Fprintln(w, "# HELP lanpaper_http_request_duration_bucket Requests in bounded latency buckets.")
	fmt.Fprintln(w, "# TYPE lanpaper_http_request_duration_bucket counter")
	labels := []string{"0.01", "0.05", "0.1", "0.5", "1", "+Inf"}
	for i := range latency {
		fmt.Fprintf(w, "lanpaper_http_request_duration_bucket{le=\"%s\"} %d\n", labels[i], latency[i].Load())
	}
	fmt.Fprintf(w, "lanpaper_active_uploads %d\nlanpaper_processing_queue 0\nlanpaper_rate_limit_rejections_total %d\nlanpaper_active_sessions %d\nlanpaper_persistence_failures_total %d\nlanpaper_disk_usage_bytes %d\n", activeUploads.Load(), rateRejected.Load(), sessionCount, persistenceFailures.Load(), diskBytes)
}
