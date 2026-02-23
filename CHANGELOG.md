# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed
- Rate limit counters are now isolated per endpoint group (`public` vs `upload`),
  preventing upload activity from consuming the public rate limit for the same IP
- `MaybeBasicAuth` now evaluates `DisableAuth` per-request instead of once at
  middleware registration (closure bug)
- `isValidLinkName` regex is now compiled once at startup (`regexp.MustCompile`)
  instead of on every request
- `public.go` now uses `http.ServeContent` instead of `http.ServeFile` so that
  manually set headers (`Cache-Control`, `Content-Type`, `Content-Disposition`)
  are not silently overwritten
- `ImagePath` / `PreviewPath` are no longer persisted to `wallpapers.json`
  (tagged `json:"-"`); they are derived at load time via `derivePaths()`
- Removed `Cross-Origin-Embedder-Policy: require-corp` header that broke
  loading of external images in the admin panel
- `validate()` extracted from `Load()` and made package-level so tests can call
  it directly without triggering env/file I/O

### Added
- `validate()` function in `config/config.go` — sanitises port, upload limits,
  rate limits, proxy type, and auto-disables auth when no credentials are set
- Config validation unit tests (`config_test.go`) covering port, MaxUploadMB,
  MaxConcurrentUploads, rate limits, proxy type, and auth auto-disable logic
- Startup log warnings when auth is auto-disabled or `DISABLE_AUTH=true`
- `uptime` field in `/health` response

### Changed
- Unified environment variable naming:
  - `PROXY_USER` → `PROXY_USERNAME` (old name still accepted as fallback)
  - `PROXY_PASS` → `PROXY_PASSWORD` (old name still accepted as fallback)
  - `RATE_LIMIT` removed; replaced by `RATE_PUBLIC_PER_MIN`, `RATE_UPLOAD_PER_MIN`, `RATE_BURST`
- `config.example.json` field names aligned with Go struct tags:
  - `username` → `adminUser`, `password` → `adminPass`
  - `public_per_min` / `admin_per_min` → `publicPerMin` / `uploadPerMin`
- `docker-compose-example.yml` updated: proxy section commented out by default,
  `PORT` set to `8080` inside container, all rate limit vars explicit
- README rewritten: env variable reference table, removed stale `config.json`
  Quick Start steps, accurate auth behaviour docs

### Removed
- `Cross-Origin-Embedder-Policy: require-corp` security header (incompatible
  with external image loading without CORP headers on remote servers)
- `admin_per_min` rate limit field (admin endpoints are not separately rate-limited)

---

## [0.8.0] - 2026-02-08

### Security

#### Magic Bytes Validation
- Deep file type verification using magic bytes signatures
- Protection against file extension spoofing (e.g., malware.exe renamed to image.jpg)
- Supports all formats: JPEG, PNG, GIF, WebP, BMP, TIFF, MP4, WebM
- Special handling for complex formats (WebP RIFF, MP4 ftyp box)

#### Enhanced Security Headers
- Strict Content Security Policy (no `unsafe-inline` for scripts)
- `X-Download-Options: noopen`
- `Cross-Origin-Resource-Policy: same-origin`
- `Cross-Origin-Opener-Policy: same-origin`
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy: geolocation=(), microphone=(), camera=()`

#### Path Traversal Protection
- `IsValidLocalPath()` with null-byte, absolute path, `..`, and UNC path checks
- Double validation via `filepath.Abs()` — path must stay within base directory
- All traversal attempts logged

#### Upload Security
- Content-Length validation before reading body
- MIME type whitelist
- Filename sanitization
- Magic bytes validation for all uploaded files

### Reliability
- Graceful shutdown with 30-second timeout (`SIGTERM` / `SIGINT`)
- HTTP server timeouts: Read 30s, Write 30s, Idle 120s
- Atomic writes for `wallpapers.json` (temp file + rename)
- All previously ignored errors now handled with contextual logging

### Features
- `/health` endpoint returning JSON status, version, and timestamp
- Video upload support (MP4, WebM) with separate copy helpers
- `PruneOldImages()` for automatic cleanup when `MAX_IMAGES` is set
- Authentication auto-disabled when `ADMIN_USER` / `ADMIN_PASS` are not set

### Architecture
- `main.go` reduced from 900+ to ~80 lines
- Split into modules: `config`, `handlers`, `middleware`, `storage`, `utils`

## [0.7.7] - 2024-XX-XX

### Added
- Basic image upload functionality
- Video support (MP4, WebM)
- Admin panel
- Basic Auth
- Rate limiting
- Docker support
- Proxy for image downloads

---

**Format:** [Semantic Versioning](https://semver.org/)
**Changelog:** [Keep a Changelog](https://keepachangelog.com/)
