// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"lanpaper/config"
	"lanpaper/handlers"
	"lanpaper/middleware"
	"lanpaper/storage"

	"github.com/joho/godotenv"
)

// Version is injected at build time via -ldflags "-X main.Version=..."; falls back to "dev".
var Version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "audit" {
		os.Exit(runAudit(os.Args[2:], os.Stdout, os.Stderr))
	}
	if len(os.Args) == 2 && os.Args[1] == "hash-password" {
		password, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && password == "" {
			log.Fatalf("Read password from stdin: %v", err)
		}
		hash, err := middleware.GeneratePasswordHash(strings.TrimRight(password, "\r\n"))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(hash)
		return
	}
	_ = godotenv.Load()
	config.Load()

	if config.Current.DisableAuth {
		log.Println("Warning: admin authentication explicitly disabled — protect /admin and /api/* at the reverse proxy.")
	} else if config.Current.AdminUser == "" || (config.Current.AdminPasswordHash == "" && config.Current.AdminPass == "") {
		log.Println("Warning: admin credentials missing; admin endpoints will return 503. Set ADMIN_USER and ADMIN_PASSWORD_HASH (or migration-only ADMIN_PASS) or explicitly set DISABLE_AUTH=true behind an auth proxy.")
	}

	handlers.InitUploadSemaphore(config.Current.MaxConcurrentUploads)

	for _, d := range []string{"data", config.MediaDir, config.PreviewDir} {
		if err := os.MkdirAll(d, config.DataDirPerm); err != nil {
			log.Fatalf("Cannot create data directory %s: %v", d, err)
		}
	}
	// Playlist items always get their directory; archived versions only when
	// history is enabled. Both are also created on demand, so a failure here is
	// a warning and not a reason to refuse to start.
	extraDirs := []string{config.ItemsDir}
	if config.Current.History.Limit > 0 {
		extraDirs = append(extraDirs, config.HistoryDir)
	}
	for _, d := range extraDirs {
		if err := os.MkdirAll(d, config.DataDirPerm); err != nil {
			log.Printf("Warning: cannot create %s: %v", d, err)
		}
	}
	// The server gallery is optional (and often a read-only mount), so a
	// failure here is not fatal.
	if err := os.MkdirAll(config.Current.ExternalImageDir, 0755); err != nil {
		log.Printf("Warning: external gallery unavailable: %v", err)
	}
	if err := storage.Global.Load(); err != nil {
		log.Fatalf("Cannot load wallpapers (refusing to overwrite metadata): %v", err)
	}
	// Move any leftover files from static/images into data/media.
	storage.MigrateMediaToDataDir()
	// Admin sessions survive a restart. A damaged sessions file only costs a
	// new sign-in, so it is reported rather than stopping the service.
	if err := middleware.LoadSessions(); err != nil {
		log.Printf("Warning: admin sessions not restored, everyone must sign in again: %v", err)
	}

	go middleware.StartCleaner()

	port := config.Current.Port
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}

	// Refusing to start beats silently serving plaintext to an operator who
	// asked for TLS: a half-configured certificate is always a mistake.
	if config.TLSMisconfigured() {
		log.Fatal("TLS_CERT_FILE and TLS_KEY_FILE must be set together; refusing to serve plain HTTP")
	}
	tlsEnabled := config.TLSEnabled()

	srv := &http.Server{
		Addr:    port,
		Handler: newHandler(),
		// Slow-header clients are cut off early. ReadTimeout/WriteTimeout
		// bound every request; the admin upload and preview-regeneration
		// handlers extend their own deadlines after authentication.
		ReadHeaderTimeout: time.Duration(config.HTTPReadHeaderTimeout) * time.Second,
		ReadTimeout:       time.Duration(config.HTTPReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(config.HTTPWriteTimeout) * time.Second,
		IdleTimeout:       time.Duration(config.HTTPIdleTimeout) * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MB
	}
	if tlsEnabled {
		// Explicit floor: TLS 1.0/1.1 are still accepted by some stacks when
		// the field is left at its zero value. Cipher suites and curves stay at
		// Go's defaults, which are kept current and reject the weak ones.
		srv.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	log.Printf("Lanpaper %s on %s (%s, max upload %d MB, compression: %d%% quality, %d%% scale)",
		Version, port, scheme, config.Current.MaxUploadMB, config.Current.Compression.Quality, config.Current.Compression.Scale)
	log.Printf("Admin: %s://localhost%s/admin", scheme, port)

	serveErr := make(chan error, 1)
	go func() {
		if tlsEnabled {
			serveErr <- srv.ListenAndServeTLS(config.Current.TLSCertFile, config.Current.TLSKeyFile)
			return
		}
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Server error: %v", err)
		}
	case <-ctx.Done():
		stop() // a second signal terminates immediately
		log.Println("Shutting down...")
		// Wait for in-flight requests (uploads, metadata writes) to finish.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(config.ShutdownTimeout)*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}
	log.Println("Server stopped.")
}

// newHandler is shared with the end-to-end HTTP tests, so tests exercise the
// real compression, authentication, CSRF, panic-recovery and routing stack,
// not just bare handlers.
func newHandler() http.Handler {
	return middleware.Gzip(middleware.Recover(newMux()))
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/static/", serveStaticAsset)
	// The service worker must live at the root scope to control /admin and
	// public link URLs; it is served with Service-Worker-Allowed: /.
	mux.HandleFunc("/sw.js", serveServiceWorker)
	// Legacy URL kept working for PWA installs/bookmarks that used /admin.html.
	mux.HandleFunc("/admin.html", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin", http.StatusPermanentRedirect)
	})
	// Crawlers have no business walking mutable media links: every hit costs
	// bandwidth and an indexed URL outlives the access level it was public
	// under. The name is reserved, so no link can ever be shadowed by it.
	mux.HandleFunc("/robots.txt", serveRobotsTxt)
	// Browsers and PWA installers probe these root paths even though the panel
	// links /static/...; the names are reserved, so no link can be shadowed by
	// the alias. A temporary redirect keeps the URL usable without caching the
	// alias forever.
	mux.HandleFunc("/favicon.ico", redirectToStaticAsset("favicon.svg"))
	mux.HandleFunc("/manifest.json", redirectToStaticAsset("manifest.json"))
	mux.HandleFunc("/manifest.webmanifest", redirectToStaticAsset("manifest.json"))
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/health/ready", readyHandler)
	mux.HandleFunc("/admin", middleware.WithSecurity(middleware.AdminPage(serveAdminPage, serveLoginPage)))
	mux.HandleFunc("/api/session", middleware.WithSecurity(middleware.HandleSession))
	mux.HandleFunc("/api/sessions", middleware.WithSecurity(middleware.MaybeBasicAuth(middleware.HandleSessions)))
	mux.HandleFunc("/api/wallpapers", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.Wallpapers)))
	mux.HandleFunc("/api/compression-config", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.GetCompressionConfig)))
	mux.HandleFunc("/api/preview/", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.AdminPreview)))
	// Publish keys (PUBLISH_KEYS) are accepted on the two routes an automation
	// needs: pushing media and creating the link to push it into. Everything
	// else on these routes still requires the admin login.
	mux.HandleFunc("/api/link/", middleware.WithSecurity(middleware.PublishOrAdmin(middleware.AllowPublishCreateLink, handleLinkRoutes)))
	mux.HandleFunc("/api/link", middleware.WithSecurity(middleware.PublishOrAdmin(middleware.AllowPublishCreateLink, handlers.Link)))
	mux.HandleFunc("/api/upload",
		middleware.WithSecurity(middleware.PublishOrAdmin(middleware.AllowPublishUpload,
			middleware.RateLimit(func() (int, int) {
				return config.Current.Rate.UploadPerMin, config.Current.Rate.Burst
			})(postOnly(handlers.Upload)),
		)),
	)
	mux.HandleFunc("/api/external-images", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImages)))
	mux.HandleFunc("/api/external-image-preview", middleware.WithSecurity(middleware.MaybeBasicAuth(handlers.ExternalImagePreview)))
	mux.HandleFunc("/api/regenerate-previews",
		middleware.WithSecurity(middleware.MaybeBasicAuth(
			middleware.RateLimit(func() (int, int) {
				// Regen is CPU-heavy — reuse the upload budget.
				return config.Current.Rate.UploadPerMin, config.Current.Rate.Burst
			})(postOnly(handlers.RegeneratePreviews)),
		)),
	)
	mux.HandleFunc("/", middleware.WithPublicSecurity(middleware.PublicRateLimit(handlers.Public)))

	return mux
}

// redirectToStaticAsset answers a reserved root path with a redirect to the
// file under /static/ that actually holds it.
func redirectToStaticAsset(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/static/"+name, http.StatusFound)
	}
}

// postOnly rejects every other method before the wrapped handler runs. It sits
// inside the upload rate limiter so that a probe like GET /api/upload, which
// only ever gets a 405, does not spend the upload budget of a real uploader.
func postOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

// handleLinkRoutes dispatches the sub-resources of /api/link/{name}. Every
// branch keeps its method, so a GET to /pin or a POST to /history still ends up
// in the handler that answers 405 for it.
func handleLinkRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/pin"):
		handlers.TogglePin(w, r)
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/history"):
		handlers.LinkHistory(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/rollback"):
		handlers.RollbackLink(w, r)
	case r.Method == http.MethodDelete && strings.Contains(path, "/history/"):
		handlers.DeleteHistoryVersion(w, r)
	default:
		handlers.Link(w, r)
	}
}

// healthHandler is the liveness probe: it answers as long as the process can
// serve HTTP, without touching the disk.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	if !isReadMethod(r) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// The version is deliberately not reported: an unauthenticated probe should
	// not learn which release to look up for known flaws.
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "lanpaper",
	})
}

// readyHandler is the readiness probe: it fails while the data directories are
// unreachable or the disk is nearly full, so an orchestrator stops sending
// traffic instead of serving 500s.
func readyHandler(w http.ResponseWriter, r *http.Request) {
	if !isReadMethod(r) {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	type check struct {
		OK      bool   `json:"ok"`
		Message string `json:"message,omitempty"`
	}

	checks := make(map[string]check, 3)
	ready := true

	for _, entry := range []struct{ key, dir string }{
		{"storage", "data"},
		{"media", config.MediaDir},
	} {
		if _, err := os.Stat(entry.dir); err != nil {
			checks[entry.key] = check{OK: false, Message: entry.dir + " not accessible"}
			ready = false
		} else {
			checks[entry.key] = check{OK: true}
		}
	}

	// Check disk space
	if freeGB, err := getDiskFreeGB("."); err != nil {
		checks["disk"] = check{OK: false, Message: "cannot check disk space"}
		ready = false
	} else if freeGB < 1 {
		checks["disk"] = check{OK: false, Message: "low disk space"}
		ready = false
	} else {
		checks["disk"] = check{OK: true}
	}

	code := http.StatusOK
	status := "ready"
	if !ready {
		code = http.StatusServiceUnavailable
		status = "not ready"
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "checks": checks})
}

// getDiskFreeGB returns free disk space in GB for the given path.
func getDiskFreeGB(path string) (float64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	return float64(freeBytes) / (1024 * 1024 * 1024), nil
}

// isReadMethod reports whether a request only reads (GET or HEAD). Probes and
// static answers reject everything else with 405 instead of doing work for a
// method they do not implement.
func isReadMethod(r *http.Request) bool {
	return r.Method == http.MethodGet || r.Method == http.MethodHead
}
