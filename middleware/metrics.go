// SPDX-License-Identifier: MIT

package middleware

import (
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"lanpaper/config"
	appmetrics "lanpaper/internal/metrics"
)

type metricResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *metricResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(p)
}
func (w *metricResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		mw := &metricResponseWriter{ResponseWriter: w}
		next.ServeHTTP(mw, r)
		status := mw.status
		if status == 0 {
			status = http.StatusOK
		}
		appmetrics.ObserveRequest(status, time.Since(start))
	})
}

func MetricsEndpoint(w http.ResponseWriter, r *http.Request) {
	if !config.Current.MetricsEnabled {
		http.NotFound(w, r)
		return
	}
	MaybeBasicAuth(HandleMetrics)(w, r)
}

var diskMetricCache struct {
	sync.Mutex
	at    time.Time
	bytes int64
}

func diskUsageMetric(now time.Time) int64 {
	diskMetricCache.Lock()
	defer diskMetricCache.Unlock()
	if now.Sub(diskMetricCache.at) < 30*time.Second {
		return diskMetricCache.bytes
	}
	var bytes int64
	_ = filepath.WalkDir("data", func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				bytes += info.Size()
			}
		}
		return nil
	})
	diskMetricCache.at, diskMetricCache.bytes = now, bytes
	return bytes
}

func HandleMetrics(w http.ResponseWriter, r *http.Request) {
	if !config.Current.MetricsEnabled {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	appmetrics.WritePrometheus(w, ActiveSessionCount(), diskUsageMetric(time.Now()))
}

func ActiveSessionCount() int { return sessionStore.ActiveCount(time.Now()) }
