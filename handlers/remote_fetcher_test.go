// SPDX-License-Identifier: MIT

package handlers

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func TestRemoteFetcherUsesInjectedTransportClockAndTempDir(t *testing.T) {
	seen := false
	fetcher := &RemoteFetcher{Clock: fixedClock{time.Now()}, TempDir: t.TempDir(), Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seen = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("media")), ContentLength: 5, Header: make(http.Header), Request: r}, nil
	})}
	file, n, err := fetcher.Fetch(t.Context(), "https://public.example/image", 16)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if !seen || n != 5 {
		t.Fatalf("seen=%v bytes=%d", seen, n)
	}
	body, err := io.ReadAll(file)
	if err != nil || string(body) != "media" {
		t.Fatalf("body=%q err=%v", body, err)
	}
}
