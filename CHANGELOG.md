# Changelog

Notable changes to Lanpaper. Docker images are published as
`ptabi/lanpaper:<version>` and `ptabi/lanpaper:latest`.

## [0.12.0] – 2026-10-03

Everything in this release is off by default or backwards compatible: an
existing `data/` directory, an existing `wallpapers.json` and every existing URL
keep working unchanged, and a link that uses none of the new features is stored
and served exactly as before.

### Added

- **Version history and rollback.** Replacing a file archives the one it
  replaced in `data/history/{link}/{version}.{ext}`. Archived versions stay
  reachable as `/{name}?v=2`, are listed by `GET /api/link/{name}/history`, can
  be restored with `POST /api/link/{name}/rollback` and dropped one by one with
  `DELETE /api/link/{name}/history/{version}`. A rollback archives the file it
  displaces, so restoring version 2 of a link at version 4 makes the live file
  version 5 and keeps version 4 — a rollback can always be rolled back. The
  thumbnail is regenerated from the restored file.
  - Defaults: `HISTORY_LIMIT=3` versions per link and `HISTORY_MAX_MB=512` as a
    budget shared by all links. `HISTORY_LIMIT=0` restores the previous
    replace-only behaviour and writes no archive at all.
  - When the budget is exceeded, the oldest archives across every link are
    dropped first, in the background pass that already pruned media. The pass
    recomputes the total from the metadata before deciding, so a missed counter
    update heals itself.
  - Renaming a link moves its archive; deleting a link deletes it.
- **Playlists and rotation.** One URL can now serve several files.
  `POST /api/upload` with `mode=append` stores a file as a playlist item in
  `data/items/{link}/{id}.{ext}` and leaves the live file untouched, so every
  existing embed keeps showing what it showed. Items are addressable as
  `/{name}?i=2` (`?i=0` is always the live file) and removable with
  `PATCH /api/link/{name}` and `{"removeItem":2}`. `PLAYLIST_MAX` (default 8,
  maximum 64) caps the list and answers `409` when it is full.
  - `PATCH /api/link/{name}` also accepts
    `{"rotate":{"enabled":true,"interval":300,"order":"random"}}`. Fields are
    merged, so toggling `enabled` keeps the interval and the order;
    `interval: 0` restores the default of 60 s, and any other value must be
    5–86400 s.
  - Rotation is **stateless**: the position is derived from
    `floor(now / interval)` (sequential) or a stable FNV hash of the link name
    and that window (random). No goroutine, no persisted cursor, no extra
    request-time I/O, the image cannot flicker inside one window, and two
    replicas reading the same `data/` directory always agree.
  - Playlist items get no thumbnail on purpose: a WebP per item would multiply
    upload CPU and disk for metadata nobody looks at.
- **Publishing without the admin login.** `PUBLISH_KEYS` (comma-separated,
  16+ characters, at most 32 keys) authorizes `POST /api/upload` and
  `POST /api/link` only, via `X-Api-Key` or `Authorization: Bearer`. Renaming,
  re-scoping, pinning, listing, rolling back and deleting still need Basic Auth,
  so a leaked key can publish but cannot read or destroy the library.
- **`autoCreate=1` on upload** creates the link and pushes its first file in one
  request — what a webhook or a cron job needs. Optional `category` and
  `accessLevel` fields are validated before the link is created. Without the
  flag an unknown name is still `400`, so a typo cannot silently create a link.
- **CORS for public media.** `CORS_ORIGINS` (comma-separated, `*` for any
  origin) adds `Access-Control-Allow-Origin`, `Vary: Origin` and
  `Access-Control-Expose-Headers` (`ETag`, `Last-Modified`, `Content-Length`,
  `Content-Range`, `Content-Type`) and answers `OPTIONS` preflights with `204`,
  `Access-Control-Allow-Methods: GET, HEAD, OPTIONS`, `Max-Age: 600` and the
  requested headers filtered to an allowlist. A browser canvas or `fetch()` on
  another origin can finally read a public link.
- **`ALLOW_EMBED=true`** drops `X-Frame-Options` and the CSP `sandbox` for
  public media only, so a kiosk page or a dashboard can frame `/{name}`.
  `/admin` and `/api/*` stay unframable whatever this flag says.
- **URL aliases.** `/{name}.jpg`, `/{name}.png`, `/{name}/latest` and
  combinations of the two resolve to `/{name}`, for clients that insist on a
  file extension. Only extensions from the supported media list are aliases, so
  `/manifest.json`, `/favicon.ico`, `/robots.txt` and `/sw.js` keep answering
  `404`. The stored extension still decides `Content-Type` and
  `Content-Disposition`, never the alias.
- **Per-link access counters.** `stats` in the link object reports requests,
  bytes and the last hit since the process started, in memory only: no visitor
  identity, no storage, no cost after a restart. `304` and `HEAD` count as a hit
  with zero bytes; refused requests are not counted.
- **Admin panel:** a per-link *Versions* dialog lists archived versions
  (open / restore / delete), playlist items (open / remove), rotation settings
  and the access counters. The upload menu gained *Add to playlist*, and a card
  shows `v5 · 2 in playlist · rotating · 42 hits` only when the link uses those
  features.

### Changed

- `PATCH /api/link/{name}` with `newLinkName` moves `data/history/{name}/` and
  `data/items/{name}/` along with the media, and rolls the renames back if the
  metadata save fails. `DELETE /api/link/{name}` removes both directories and
  the link's counters.
- `data/items/` is created at startup, `data/history/` only when history is
  enabled. Both are also created on demand, so a failure to create them is a
  warning and not a reason to refuse to start.
- The service worker cache is bumped to `lanpaper-static-v5` so clients pick up
  the new panel assets.

### Security

- Publish keys are compared as SHA-256 digests in constant time, are never
  written to `config.json` and appear in logs only as an 8-hex-character
  fingerprint. A wrong key shares the brute-force budget with admin logins
  (10 failures per client per 15 minutes, then `429` with `Retry-After`), and
  the lockout is not an oracle: the right key is refused while it lasts. On a
  server without `PUBLISH_KEYS` a stray `X-Api-Key` header is ignored entirely
  and cannot lock anybody out.
- `CORS_ORIGINS` entries are parsed as URLs: a scheme of `http`/`https`, no path,
  no credentials and no wildcard subdomains. An origin that is not listed gets
  no CORS headers at all, and `OPTIONS` keeps answering `405` until CORS is
  configured, so the default deployment behaves exactly as before.
- An unusable `?v=` or `?i=` selector is a `404` instead of a silent fallback to
  other bytes, and the access check of the link always runs before any selector
  is resolved, so an alias or a version cannot bypass `token`, `local` or `auth`.
- Archived versions and playlist items are stored under validated link names and
  validated extensions only, in the same `data/` tree as the live media: nothing
  new is reachable from the static web root.

### Performance

- Archiving is a **rename** of the backup that an upload already created, so a
  replacement costs no extra copy and no extra disk read.
- Rotation and version selection are pure arithmetic on data that is already in
  memory; a request for `/{name}` does the same single `ServeContent` call as
  before, and media still uses `sendfile` (the stats counter wraps the response
  writer with a `ReadFrom` delegate).
- Playlist appends skip thumbnail generation, and the item cap is checked before
  any media is downloaded or decoded.
- The history budget runs in the existing prune worker instead of adding a
  timer, and the byte counter is a single atomic.

### Production readiness

- **A panicking handler costs one request, not the process.** A recovery layer
  between the gzip middleware and the router logs the method, the path and the
  stack, and answers a clean `500` when the response has not started yet.
  Nothing from the panic reaches the client, and the query string is never
  logged because token links carry their secret there. `http.ErrAbortHandler`
  is re-raised untouched, so a deliberately abandoned response still behaves
  the way `net/http` documents. The two background goroutines nobody restarts
  (the prune worker and the rate-limit cleaner) recover as well.
- **Optional built-in TLS.** `TLS_CERT_FILE` + `TLS_KEY_FILE` serve HTTPS from
  the process itself with a TLS 1.2 floor; Go's current cipher suite and curve
  defaults apply, and HTTP/2 is negotiated automatically. Setting only one of
  the two is a startup error: refusing to start beats silently serving admin
  credentials and token URLs in plaintext.
- **`/robots.txt`** answers `Disallow: /` instead of `404`, so crawlers stop
  walking mutable media URLs (and stop retrying a path that will never exist).
- **The probes reject write methods** with `405` instead of answering a `POST`
  to `/health` as if it were a `GET`.
- **Startup warnings** for `INSECURE_SKIP_VERIFY=true`, `CORS_ORIGINS=*` and an
  `ADMIN_PASS` shorter than 12 characters, so an internet-facing mistake is
  visible in the log on day one.
- **Package documentation** for `main`, `config`, `storage`, `handlers`,
  `middleware` and `utils`, spelling out the invariants each package relies on
  (copy-on-write snapshots, lock ordering, transparent response writers, no
  global reads on the request path).
- **Licence hygiene:** every first-party source file carries an
  `SPDX-License-Identifier: MIT` header, and
  [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) lists all bundled
  components (BSD-3-Clause, MIT, SIL OFL 1.1 — no copyleft) with the texts a
  redistribution must reproduce.
- **Release automation:** pushing a version tag runs the full test matrix,
  publishes `linux/amd64` + `linux/arm64` images under the semver tags and
  creates the GitHub release, after checking that the tag matches `VERSION`.
- **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)** documents the production
  topology, a hardened systemd unit, log lines worth alerting on, capacity
  planning, backups and a pre-launch checklist.

## [0.11.0] – 2026-09-29

### Changed

- **New admin panel design.** An image-first gallery look: large 16:9
  previews in framed tiles, a sticky glass toolbar, rounded pill controls and
  a sunset gradient accent, with matching light and dark themes.
- **New app icon, favicon and logo**: a sunset between two peaks with signal
  arcs. SVG favicon and logo; PNG icons at 192 and 512 px, a maskable 512 px
  icon and a 180 px Apple touch icon.
- **Fonts:** Manrope for the interface (Latin and Cyrillic) and Unbounded for
  link IDs, as WOFF2 subsets with their OFL licences: 48 KB instead of 528 KB
  of TTF. Rubik and the Disket Mono fonts, one of which was never used, are
  removed. The service worker now precaches 211 KB instead of 713 KB.
- **Mobile toolbar:** the search field has its own row; the result counter
  and the sort menu share the next row instead of squeezing into the search
  field.

### Added

- **Previews for icons and unusual aspect ratios.** Small images, and square
  images in formats with transparency (PNG, WebP, GIF), are treated as icons:
  they are shown at their own size on a blurred copy of themselves instead of
  being cropped and enlarged. Portrait, square and panoramic images are shown
  whole, and portrait videos are letterboxed. The server gallery uses the same
  rules.
- **gzip compression** for text responses of at least 1 KiB (HTML, CSS,
  JavaScript, JSON, SVG). The admin page downloads about three times less, and
  a 1,000-link API response shrinks from about 295 KB to 26 KB. Media, range
  requests and `HEAD` responses are never compressed, so media keeps the
  `sendfile` path.

### Performance

- **Uploads use far less memory.** Images are resized without a full-size
  intermediate buffer, and memory is returned to the system as soon as no
  image is being processed. The process drops back to about 10 MB right after
  an upload instead of staying at its peak. Peak memory, before → after:

  | Upload | Default | `COMPRESSION_SCALE=50` |
  | --- | --- | --- |
  | 4K JPEG | 65 → 23 MB | 179 → 31 MB |
  | 24 MP JPEG | 114 → 48 MB | 470 → 71 MB |
  | 36 MP JPEG | 144 → 65 MB | 689 → 99 MB |
  | Two 24 MP JPEGs at once | 218 → 86 MB | 932 → 132 MB |

  Uploads take 5–60 % longer; memory was the priority.
- **Browser caching.** Admin previews and `public`/`local` media are
  revalidated (`Cache-Control: private, no-cache` with `ETag` and
  `Last-Modified`) instead of downloaded again. The access check still runs
  on every request, before any `304`. A repeat visit to a 13-link panel
  transfers about 12 KB instead of 890 KB. `token` and `auth` media stays
  `no-store`.
- **Video previews play only while visible** (at least a quarter on screen,
  tab visible) and never with reduced motion. Offscreen videos are not
  downloaded: the first visit on a phone transfers about 106 KB instead of
  1.36 MB.

### Fixed

- The theme and view toggle icons animate again; the stylesheet targeted
  button IDs that did not exist.
- Long link names end with an ellipsis instead of being cut off.
- "Copy URL" falls back to the legacy clipboard method when the browser
  refuses `navigator.clipboard` (permission policy, unfocused page).
- Server gallery tiles no longer fade in before the file has loaded.

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
