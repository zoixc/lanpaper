package handlers

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"lanpaper/config"
	"lanpaper/utils"
)

// safeRemoteTransport resolves EACH redirect target and connects to the
// vetted IP rather than handing its hostname to a proxy (or resolving it a
// second time at dial). The original Host header and TLS SNI are preserved.
// A new transport per hop also prevents cross-host reuse of a TLS connection.
type safeRemoteTransport struct{}

func (safeRemoteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ip, err := utils.ResolvePublicURL(req.Context(), req.URL.String())
	if err != nil {
		return nil, err
	}
	port := req.URL.Port()
	if port == "" {
		if req.URL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	pinned := req.Clone(req.Context())
	urlCopy := *req.URL
	urlCopy.Host = net.JoinHostPort(ip.String(), port)
	pinned.URL = &urlCopy
	pinned.Host = req.URL.Host // virtual hosting, even through an HTTP proxy

	insecure := config.Current.InsecureSkipVerify // explicitly configured opt-in
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		DialContext:            dialer.DialContext,
		TLSClientConfig:        &tls.Config{ServerName: req.URL.Hostname(), InsecureSkipVerify: insecure},
		TLSHandshakeTimeout:    10 * time.Second,
		ResponseHeaderTimeout:  30 * time.Second,
		MaxResponseHeaderBytes: 1 << 20,
		DisableKeepAlives:      true,
	}
	if host := config.Current.ProxyHost; host != "" {
		proxy := &url.URL{Scheme: config.Current.ProxyType, Host: net.JoinHostPort(host, config.Current.ProxyPort)}
		if (proxy.Scheme == "http" || proxy.Scheme == "https") && req.URL.Scheme == "http" {
			// Go's Request.WriteProxy uses Request.Host for BOTH the Host
			// header and the authority in the absolute request URI. Merely
			// changing URL.Host to the vetted IP while preserving the virtual
			// Host would therefore let the proxy re-resolve the original name
			// (DNS rebinding SSRF). An opaque URL makes RequestURI return the
			// pinned absolute URI, independently of the original Host header.
			path := urlCopy.EscapedPath()
			if path == "" {
				path = "/"
			}
			urlCopy.Opaque = "//" + urlCopy.Host + path
		}
		if config.Current.ProxyUsername != "" {
			proxy.User = url.UserPassword(config.Current.ProxyUsername, config.Current.ProxyPassword)
		}
		tr.Proxy = http.ProxyURL(proxy)
		if proxy.Scheme == "https" {
			// Transport's TLSClientConfig is for the TARGET hostname (SNI after
			// CONNECT). The HTTPS proxy needs a separate TLS handshake with its
			// own hostname; otherwise certificate verification would fail.
			tr.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr != proxy.Host {
					return nil, errors.New("unexpected proxy address")
				}
				conn, err := dialer.DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				c := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: insecure})
				if err := c.HandshakeContext(ctx); err != nil {
					conn.Close()
					return nil, err
				}
				return c, nil
			}
		}
	}
	resp, err := tr.RoundTrip(pinned)
	if err != nil {
		tr.CloseIdleConnections()
		return nil, err
	}
	return resp, nil
}

// downloadToTemp streams network responses to disk, not a MaxUploadMB-sized
// byte slice in RAM. The caller closes and removes the returned temp file.
func downloadToTemp(ctx context.Context, raw string, maxBytes int64) (*os.File, int64, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(config.DownloadTimeout)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Lanpaper/1.0")
	req.Header.Set("Accept", "image/*,video/mp4,video/webm;q=0.8")
	client := &http.Client{
		Transport: safeRemoteTransport{},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return errors.New("too many redirects")
			}
			// RoundTrip will resolve and pin the next target before any I/O.
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return errors.New("redirect scheme not allowed")
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, 0, errMediaTooLarge
	}
	f, err := os.CreateTemp(config.MediaDir, ".download-*")
	if err != nil {
		return nil, 0, err
	}
	fail := func(err error) (*os.File, int64, error) {
		f.Close()
		os.Remove(f.Name())
		return nil, 0, err
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return fail(err)
	}
	if n > maxBytes {
		return fail(errMediaTooLarge)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return f, n, nil
}
