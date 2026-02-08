# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Security Enhancements

#### Magic Bytes Validation
- Deep file type verification using magic bytes signatures
- Protection against file extension spoofing (e.g., malware.exe renamed to image.jpg)
- Validates actual file content matches declared extension
- Supports all formats: JPEG, PNG, GIF, WebP, BMP, TIFF, MP4, WebM
- Special handling for complex formats (WebP RIFF, MP4 ftyp)
- Minimal performance overhead (~0.01ms per file)

#### Enhanced Security Headers
- `X-Download-Options: noopen` - prevents IE from opening downloads in site context
- `Cross-Origin-Resource-Policy: same-origin` - CORP protection against Spectre attacks
- `Cross-Origin-Embedder-Policy: require-corp` - COEP isolation
- `Cross-Origin-Opener-Policy: same-origin` - COOP protection

#### Upload Security
- Content-Length validation before reading body (bomb protection)
- MIME type validation with whitelist
- Filename sanitization for safe logging
- Enhanced security logging for all rejected uploads
- Protection against zip bombs and size attacks

## [0.8.0] - 2026-02-08

### Security

#### Enhanced Content Security Policy
- Removed `unsafe-inline` for scripts - inline JavaScript now prohibited
- Added `form-action 'self'` - protection against form submissions to external sites
- Added `base-uri 'self'` - protection against base URL manipulation
- Added `frame-ancestors 'none'` - clickjacking protection
- Added `blob:` support for media/img - for blob URLs

#### Path Traversal Protection
- New `IsValidLocalPath()` function with checks for:
  - Null bytes (`\x00`)
  - Absolute paths
  - Directory traversal attempts (`..`)
  - UNC paths on Windows (`\\`)
- Double validation via `filepath.Abs()` - path must stay within base directory
- Logging of all path traversal attempts

#### Additional Security Headers
- `X-XSS-Protection: 1; mode=block`
- `Referrer-Policy: strict-origin-when-cross-origin`
- `Permissions-Policy: geolocation=(), microphone=(), camera=()`

#### Input Validation
- File size check from header before processing
- Content-type validation for uploaded files
- Logging of rejected files

#### Security Logging
- `"Security: blocked invalid path attempt"`
- `"Security: blocked path traversal attempt"`
- `"Failed authentication attempt from IP"`
- `"Rate limit exceeded for IP"`
- `"Rejected unsupported content type"`

### Reliability

#### Graceful Shutdown
- Proper handling of `SIGTERM` and `SIGINT` signals
- 30-second timeout for completing current requests
- Shutdown process logging

#### HTTP Server Timeouts
- `ReadTimeout: 30s` - protection against slow clients
- `WriteTimeout: 30s` - response time limitation
- `IdleTimeout: 120s` - automatic closure of idle connections

#### Improved Error Handling
All previously ignored errors (`_`) are now handled:
- `json.Marshal/Encode` - serialization error logging
- `os.Remove` - check for `os.IsNotExist` before logging
- `os.Stat` - return HTTP 500 on errors
- `io.Copy` - copy error logging
- `file.Seek` - positioning error handling

#### Contextual Logging
All logs now include context:
- `"Error saving wallpapers after upload: %v"`
- `"Error removing old image %s: %v"`
- `"Failed to create directory %s: %v"`
- `"Error stating uploaded file %s: %v"`

### Quality of Life

- Authentication automatically disabled if `ADMIN_USER` and `ADMIN_PASS` are not set
- No need to manually set `DISABLE_AUTH=true` for setups behind external auth
- Warning logged when auth is auto-disabled

### Architecture

#### Module Refactoring
Main file `main.go` reduced from 900+ to 80 lines. Code split into:

**config/**
- `config.go` - Configuration loading and storage

**handlers/**
- `admin.go` - Admin API (wallpapers, links, external images)
- `upload.go` - File upload logic (upload, download, local)
- `public.go` - Public image access
- `common.go` - Common utilities (name validation)

**middleware/**
- `auth.go` - Basic Authentication
- `security.go` - Security headers and CSP
- `ratelimit.go` - Rate limiting with cleanup

**storage/**
- `wallpaper.go` - Data storage with Get/Set/Delete/GetAll methods
- `PruneOldImages()` function for automatic cleanup

**utils/**
- `validation.go` - Path validation and file type verification

#### Benefits
- Better code readability
- Simplified testing
- Isolated changes
- Explicit dependencies via imports

### Documentation
- Updated README.md with new features description
- Added modular structure documentation
- Improved security recommendations
- Added this CHANGELOG

### Technical Improvements
- Fixed all Go compiler warnings
- Removed unused imports
- Consistent code formatting
- Correct module paths in imports

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
