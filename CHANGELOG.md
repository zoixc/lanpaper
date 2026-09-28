# Changelog

Notable changes to Lanpaper. Docker images are published as
`ptabi/lanpaper:<version>` and `ptabi/lanpaper:latest`.

## [0.10.0] – 2026-09-28

### Security

- **Brute-force protection now works.** Before, the limit only changed the
  status of *wrong* guesses to 429, and a correct password was still accepted.
  An attacker could keep guessing, and admin-protected public links were an
  unthrottled password oracle. Now:
  - After 10 failed logins per client in 15 minutes, credentials are no longer
    evaluated until the window ends (`429` + `Retry-After`).
  - The admin API and `token`/`auth` links share the counter.
  - Requests without credentials are not counted.
- **Dependency CVE fixed.** `golang.org/x/image` updated to v0.46.0, which
  fixes CVE-2026-46603 / GO-2026-6222: excessive memory allocation in the
  VP8L (lossless WebP) decoder, reachable through uploads.
- Rate limits and the lockout group IPv6 clients by `/64`, so rotating
  addresses no longer escapes them. Every `429` carries `Retry-After`.
- More special-purpose ranges are blocked for URL downloads: IPv4-compatible
  IPv6, the whole `2001::/23` block, discard-only `100::/64`, site-local
  `fec0::/10` and the 6to4 relay anycast range.
- A 10 s `ReadHeaderTimeout` against slow-header clients.
- HSTS is sent on all responses to HTTPS requests, including public media.
- Unneeded `blob:` sources removed from the admin CSP (`media-src`,
  `worker-src`). Obsolete `X-Download-Options` header removed.
- Static assets are opened through `os.Root` with a check against file swaps
  between `Lstat` and `Open`.
- Browser exports no longer contain access tokens.
- Container:
  - The unprivileged user is pinned to uid 100 / gid 101 (unchanged values).
  - Application files are no longer writable by the service.
  - `-trimpath` build.
  - The Compose example drops all capabilities and sets `no-new-privileges`.
- CI:
  - Least-privilege `GITHUB_TOKEN`.
  - Actions pinned to commit SHAs.
  - `govulncheck`, `go mod verify` and a gofmt check.
  - Dependabot for Go modules, Actions and Docker images.
- Docs: the Caddy reverse-proxy example now overwrites `X-Real-IP`. Before,
  clients could spoof it and appear to be on the LAN.

### Fixed

- **Graceful shutdown.** The process used to exit as soon as shutdown began,
  cutting off running uploads. It now waits up to 30 s for in-flight requests.
- **Slow uploads.** Large uploads over slow connections were aborted by the
  30 s read timeout. After authentication, uploads now get a deadline sized to
  the maximum upload, and preview regeneration a 30-minute deadline.
- **Durability.** Metadata and media are flushed (`fsync`) before the atomic
  rename, and the metadata directory is synced, so a power loss cannot leave
  a truncated file.
- **Browser JPEG pre-processing:**
  - It used to cap JPEG uploads at 1920×1080. 4K images uploaded through the
    panel were silently downscaled, unlike URL or gallery uploads, and with
    `COMPRESSION_SCALE` < 100 they were scaled twice.
  - Dimensions are now always left to the server.
  - Pre-processing is skipped when the settings can't be loaded (so lossless
    mode is never bypassed) and for images too large for a browser canvas.
- **Copy URL** copied a stale URL after the access level changed or the token
  was rotated.
- **Mobile/tablet layout:** on token links and with long link names, the card
  text and token buttons overflowed the card.
- **Server error messages** from the API were never translated, because of
  the trailing newline in plain-text errors.
- **Create button:** its icon disappeared once translations loaded.
- **Gallery picker** shows videos (MP4/WebM) as video thumbnails instead of
  broken images. Confirming the picker now restores focus and ARIA state.
- The access-level `<label>` was not associated with its `<select>` for link
  names starting with a digit.
- The "Create new link" PWA shortcut (`/admin?action=create`) now focuses the
  input.
- Searching for a file type (e.g. `png`, `.mp4`) finds matching links.
- The configured `EXTERNAL_IMAGE_DIR` is created at startup instead of the
  hard-coded default path.
- The Docker `HEALTHCHECK` honours a custom `PORT`.

### Changed

- Go 1.26 or newer is required to build (Go 1.25 is end of life). The Docker
  build uses Go 1.27.
- Invalid numeric or boolean environment variables are logged instead of
  ignored silently.
- Admin thumbnails are encoded at a fixed WebP quality of 80. They are
  smaller in lossless mode, and stored media is unaffected.
- The gallery listing walks the directory once inside an `os.Root`, instead
  of opening a new root for every file.
- Code cleanup:
  - duplicated helpers merged: credential checks, resize, access defaults,
    static file opening;
  - Go modernizations applied with `go fix`;
  - unused i18n keys and dead frontend code removed.

### Documentation

- README, SECURITY.md and docs/API.md rewritten to match the code. This
  includes the bind-mount permissions, the lockout and the full endpoint
  reference.
- The outdated `docs/security.md` was merged into SECURITY.md. Among other
  things, it wrongly claimed that missing credentials disable authentication.
- Historical audit notes (`AUDIT.md`, `IMPROVEMENTS.md`, `UI_IMPROVEMENTS.md`)
  were replaced by this changelog. ROADMAP.md now lists only open items.

## [0.9.9] – 2026-09-27

A security-hardening series (PRs #16–#21).

- **Access and authentication.**
  - Per-link access levels (`public`, `local`, `token`, `auth`) with token
    rotation.
  - Missing admin credentials fail closed (503).
  - `DISABLE_AUTH` added as an explicit opt-out.
- **Media isolation.**
  - Media and thumbnails moved from `static/images/` to `data/media/` and
    `data/previews/` (automatic migration).
  - `/static/` serves an allowlist only, and previews are admin-only.
- **Downloads.** SSRF-safe URL downloads: every DNS answer and redirect is
  checked, the IP is pinned even through HTTP, HTTPS and SOCKS5 proxies, and
  downloads are streamed to bounded temporary files.
- **Uploads.** Content-based type detection, full decoding before
  publication, pixel and memory budgets, a streaming pure-Go WebP decoder, and
  MP4 files with any `ftyp` brand accepted.
- **Storage.** Atomic, copy-on-write metadata store with per-link locks and
  rollback. Corrupt metadata stops startup.
- **Browser protection.** CSRF protection, strict CSP and security headers,
  and a service worker limited to static assets.
- **Admin UI.**
  - Resilient initialisation on old browsers and plain-HTTP LAN access.
  - Pinning, inline rename, drag-and-drop link creation.
  - Refreshed design and PWA installability.

## [0.9.7] – 2026-02-28

See the [GitHub release](https://github.com/zoixc/lanpaper/releases/tag/0.9.7).
