package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"lanpaper/config"
)

// No external DNS/network is needed: the resolver returns two public test IPs
// and one private IP. A proxy can then confirm the actual address handed to
// the transport, including after redirects.
func testResolver() *net.Resolver {
	ips := map[string]net.IP{
		"public.test":  net.IPv4(8, 8, 8, 8),
		"second.test":  net.IPv4(1, 1, 1, 1),
		"private.test": net.IPv4(10, 0, 0, 8),
	}
	return &net.Resolver{PreferGo: true, Dial: func(_ context.Context, _, _ string) (net.Conn, error) {
		client, server := net.Pipe()
		go func() {
			defer server.Close()
			buf := make([]byte, 1500)
			// net.Pipe is a stream; Go's resolver prefixes DNS messages with
			// a two-byte length even if it requested the "udp" network.
			if _, err := io.ReadFull(server, buf[:2]); err != nil {
				return
			}
			n := int(binary.BigEndian.Uint16(buf[:2]))
			if n < 17 || n > len(buf)-2 {
				return
			}
			if _, err := io.ReadFull(server, buf[2:2+n]); err != nil {
				return
			}
			buf = buf[2 : 2+n]
			end := 12
			var labels []string
			for end < n && buf[end] != 0 {
				length := int(buf[end])
				end++
				if end+length > n {
					return
				}
				labels = append(labels, string(buf[end:end+length]))
				end += length
			}
			if end+5 > n {
				return
			}
			end++
			kind := binary.BigEndian.Uint16(buf[end : end+2])
			question := buf[12 : end+4]
			ip := ips[strings.Join(labels, ".")]
			answerCount := byte(0)
			if ip != nil && kind == 1 { // A; AAAA has no answer
				answerCount = 1
			}
			response := []byte{buf[0], buf[1], 0x81, 0x80, 0, 1, 0, answerCount, 0, 0, 0, 0}
			response = append(response, question...)
			if answerCount == 1 {
				response = append(response,
					0xc0, 0x0c, 0, 1, 0, 1, // name pointer, A, IN
					0, 0, 0, 1, 0, 4) // TTL, IPv4 length
				response = append(response, ip.To4()...)
			}
			packet := []byte{byte(len(response) >> 8), byte(len(response))}
			_, _ = server.Write(append(packet, response...))
		}()
		return client, nil
	}}
}

func setupRemoteTest(t *testing.T) {
	t.Helper()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	originalConfig, originalResolver := config.Current, net.DefaultResolver
	t.Cleanup(func() {
		config.Current = originalConfig
		net.DefaultResolver = originalResolver
		if err := os.Chdir(originalDir); err != nil {
			t.Error(err)
		}
	})
	if err := os.MkdirAll(config.MediaDir, 0755); err != nil {
		t.Fatal(err)
	}
	config.Current = config.Config{ProxyType: "http", MaxUploadMB: 2, Compression: config.CompressionConfig{Quality: 100, Scale: 100}}
	net.DefaultResolver = testResolver()
}

func configureProxy(t *testing.T, srv *httptest.Server) {
	t.Helper()
	host, port, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	config.Current.ProxyHost = host
	config.Current.ProxyPort = port
}

func readDownload(t *testing.T, raw string, max int64) ([]byte, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	file, _, err := downloadToTemp(ctx, raw, max)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	return io.ReadAll(file)
}

func TestHTTPProxyPinsHostAfterRedirect(t *testing.T) {
	setupRemoteTest(t)
	type hop struct{ url, host string }
	var mu sync.Mutex
	var seen []hop
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, hop{r.URL.String(), r.Host})
		mu.Unlock()
		switch r.URL.Host {
		case "8.8.8.8:8080":
			w.Header().Set("Location", "http://second.test:8080/picture.png")
			w.WriteHeader(http.StatusFound)
		case "1.1.1.1:8080":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte("test-download"))
		default:
			http.Error(w, "not pinned", 502)
		}
	}))
	defer proxy.Close()
	configureProxy(t, proxy)
	body, err := readDownload(t, "http://public.test:8080/first", 1024)
	if err != nil || string(body) != "test-download" {
		t.Fatalf("pinned redirect failed: %v, %q", err, body)
	}
	mu.Lock()
	defer mu.Unlock()
	// Go's net/http server derives Request.Host from the absolute-form URI,
	// ignoring the raw Host header. Inspect the wire in the next test.
	if len(seen) != 2 || seen[0] != (hop{"http://8.8.8.8:8080/first", "8.8.8.8:8080"}) ||
		seen[1] != (hop{"http://1.1.1.1:8080/picture.png", "1.1.1.1:8080"}) {
		t.Fatalf("proxy received unpinned URL: %+v", seen)
	}
}

func TestHTTPProxyWireUsesPinnedURIAndOriginalHostHeader(t *testing.T) {
	setupRemoteTest(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config.Current.ProxyHost, config.Current.ProxyPort, _ = net.SplitHostPort(listener.Addr().String())
	result := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err.Error()
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		br := bufio.NewReader(conn)
		var lines []string
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				result <- err.Error()
				return
			}
			lines = append(lines, strings.TrimSpace(line))
			if line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK")
		result <- strings.Join(lines, "\n")
	}()
	body, err := readDownload(t, "http://public.test:8080/picture.png?a=1", 1024)
	if err != nil || string(body) != "OK" {
		t.Fatalf("proxy request failed: %v %q", err, body)
	}
	raw := <-result
	if !strings.HasPrefix(raw, "GET http://8.8.8.8:8080/picture.png?a=1 HTTP/1.1\n") ||
		!strings.Contains(raw, "\nHost: public.test:8080\n") {
		t.Fatalf("proxy must get pinned URI AND original Host, got:\n%s", raw)
	}
}

func TestHTTPProxyRejectsPrivateRedirectAndOversize(t *testing.T) {
	setupRemoteTest(t)
	for _, redirect := range []string{"http://127.0.0.1/admin", "http://private.test/secret"} {
		t.Run(redirect, func(t *testing.T) {
			var mu sync.Mutex
			count := 0
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				count++
				mu.Unlock()
				http.Redirect(w, r, redirect, http.StatusFound)
			}))
			defer proxy.Close()
			configureProxy(t, proxy)
			if _, err := readDownload(t, "http://public.test/redirect", 100); err == nil {
				t.Fatalf("unsafe redirect accepted: %s", redirect)
			}
			mu.Lock()
			defer mu.Unlock()
			if count != 1 {
				t.Fatalf("proxy received %d requests, expected ONLY initial vetted request", count)
			}
		})
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", "1001")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("A"), 1001))
	}))
	defer proxy.Close()
	configureProxy(t, proxy)
	if _, err := readDownload(t, "http://public.test/large", 1000); !errors.Is(err, errMediaTooLarge) {
		t.Fatalf("expected content-length limit, got %v", err)
	}
	for _, invalid := range []string{"http://127.0.0.1/private", "http://public.test@127.0.0.1/secret", "file:///etc/passwd"} {
		if _, err := readDownload(t, invalid, 100); err == nil {
			t.Errorf("invalid URL accepted: %s", invalid)
		}
	}
	if files, _ := filepath.Glob(filepath.Join(config.MediaDir, ".download-*")); len(files) > 0 {
		t.Fatalf("temporary files leaked: %v", files)
	}
}

func TestHTTPSProxyVerifiesItsOwnCertificate(t *testing.T) {
	setupRemoteTest(t)
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "8.8.8.8:80" {
			http.Error(w, "pinning mismatch", 502)
			return
		}
		_, _ = w.Write([]byte("through HTTPS proxy"))
	}))
	defer proxy.Close()
	configureProxy(t, proxy)
	config.Current.ProxyType = "https"
	if _, err := readDownload(t, "http://public.test/image.png", 1024); err == nil {
		t.Fatal("accepted untrusted HTTPS proxy certificate")
	}
	config.Current.InsecureSkipVerify = true // explicit opt-in test only
	body, err := readDownload(t, "http://public.test/image.png", 1024)
	if err != nil || string(body) != "through HTTPS proxy" {
		t.Fatalf("HTTPS proxy handshake did not work: %v %q", err, body)
	}
}

func TestHTTPProxyConnectPinsHTTPSAndKeepsSNI(t *testing.T) {
	setupRemoteTest(t)
	var mu sync.Mutex
	var connectHost, originHost, originSNI string
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		originHost, originSNI = r.Host, r.TLS.ServerName
		mu.Unlock()
		_, _ = w.Write([]byte("TLS origin"))
	}))
	defer origin.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "CONNECT required", 405)
			return
		}
		mu.Lock()
		connectHost = r.Host
		mu.Unlock()
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer client.Close()
		upstream, err := net.Dial("tcp", strings.TrimPrefix(origin.URL, "https://"))
		if err != nil {
			t.Error(err)
			return
		}
		defer upstream.Close()
		_, _ = io.WriteString(client, "HTTP/1.1 200 Connection established\r\n\r\n")
		go func() { _, _ = io.Copy(upstream, client); _ = upstream.Close() }()
		_, _ = io.Copy(client, upstream)
	}))
	defer proxy.Close()
	configureProxy(t, proxy)
	config.Current.InsecureSkipVerify = true // httptest uses a self-signed cert
	body, err := readDownload(t, "https://public.test/photo.png", 1024)
	if err != nil || string(body) != "TLS origin" {
		t.Fatalf("HTTPS origin via proxy failed: %v %q", err, body)
	}
	mu.Lock()
	defer mu.Unlock()
	if connectHost != "8.8.8.8:443" || originHost != "public.test" || originSNI != "public.test" {
		t.Fatalf("CONNECT=%s Host=%s SNI=%s", connectHost, originHost, originSNI)
	}
}

func TestSOCKS5ProxyGetsOnlyVettedIP(t *testing.T) {
	setupRemoteTest(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	config.Current.ProxyHost, config.Current.ProxyPort, config.Current.ProxyType = host, port, "socks5"
	result := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		br := bufio.NewReader(conn)
		intro := make([]byte, 3)
		if _, err := io.ReadFull(br, intro); err != nil || intro[0] != 5 {
			result <- fmt.Errorf("SOCKS greeting: %v %v", intro, err)
			return
		}
		_, _ = conn.Write([]byte{5, 0})
		head := make([]byte, 4)
		if _, err := io.ReadFull(br, head); err != nil || head[0] != 5 || head[1] != 1 || head[3] != 1 {
			result <- fmt.Errorf("SOCKS connect header: %v %v", head, err)
			return
		}
		addr := make([]byte, 6)
		if _, err := io.ReadFull(br, addr); err != nil || !bytes.Equal(addr[:4], []byte{8, 8, 8, 8}) ||
			binary.BigEndian.Uint16(addr[4:]) != 8080 {
			result <- fmt.Errorf("SOCKS forwarded unpinned IP: %v %v", addr, err)
			return
		}
		_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
		req, err := http.ReadRequest(br)
		if err != nil || req.Host != "public.test:8080" {
			result <- fmt.Errorf("origin host incorrect: %v %v", req, err)
			return
		}
		_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Length: 10\r\n\r\nsocks done")
		result <- nil
	}()
	body, err := readDownload(t, "http://public.test:8080/image.png", 1024)
	if err != nil || string(body) != "socks done" {
		t.Fatalf("SOCKS download failed: %v %q", err, body)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
