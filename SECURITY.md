# Security policy and deployment notes

This describes the **implemented** behavior, not a guarantee that arbitrary hosting configurations are safe. Use the current build and a supported Go toolchain, and put the admin interface behind HTTPS.

## Reporting a vulnerability

Use GitHub's **Report a vulnerability** / private security advisory for this repository if available. If private reporting is not enabled, contact the maintainers through a private channel published on the repository profile. Please do not include exploit details in a public issue. Include the affected version, reproduction steps and impact; do not include real credentials or access tokens.

## Trust boundaries

- **Admin:** HTTP Basic Auth protects `/admin`, `/api/*` and private previews. Invalid credentials are rate-limited per client IP. Missing either admin credential returns **503** on admin routes, not anonymous access. `DISABLE_AUTH=true` explicitly opts out: use it only if an *independently authenticated* reverse proxy protects **both** the admin page and every admin API route, and direct access to Lanpaper's backend is blocked. There are no application sessions or hashed-password store: credentials come from configuration/environment and must be handled as secrets.
- **Public links:** `public` is accessible to all. `local` uses the client IP; `token` requires a generated secret (or admin credentials); `auth` requires admin Basic Auth. Private previews are available only via the admin API. Auth-level public URLs remain unavailable if built-in auth is disabled, even when an external proxy authenticates the admin UI.
- **Reverse proxy:** Forwarded client IP, host, scheme and TLS state are used **only** when `TRUSTED_PROXY` matches the immediate peer. Set it narrowly, and have the proxy **replace**, not pass through, client-supplied `X-Real-IP`/`X-Forwarded-*`. If the proxy appears to be a private-network client but `TRUSTED_PROXY` is not configured, every outside visitor may be classified as `local`. Also configure `Host` and forwarded scheme correctly for admin same-origin checks.
- **Tokens:** Token URLs contain a bearer secret and may appear in browser history, copied backups or reverse-proxy access logs. Avoid logging query strings; `Referrer-Policy: no-referrer` prevents browser Referer leakage. Rotate leaked tokens. Browser export files may contain tokens; they are **not** complete media backups.

## Implemented controls

| Area | Behavior |
| --- | --- |
| Admin CSRF | State-changing browser requests reject cross-site/same-site fetch metadata and a mismatched `Origin` (including port). Non-browser API clients must still authenticate. |
| Admin CSP | `default-src 'none'`; only same-origin scripts/styles/connect, explicitly allowed data/blob image/media/worker sources, and `frame-ancestors 'none'`. No inline script/style nonces are used. Public media has its own restrictive CSP. |
| Headers | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`; admin responses use COOP `same-origin`, COEP `credentialless`, CORP `same-origin` and `Cache-Control: no-store`. Public media permits cross-origin embedding and uses `no-store`. |
| HSTS | `Strict-Transport-Security: max-age=31536000` **only** for HTTPS requests (or HTTPS forwarded by a trusted proxy). No `includeSubDomains` or `preload`. Configure edge TLS/HSTS yourself if required. |
| Media access | Uploaded files are under `data/media/` and `data/previews/`; `/static/` serves only an allowlist of application assets. Files and final symlinks outside the allowed paths are not served. Legacy files under `static/images/` are migrated but **never** exposed through a static URL. |
| Upload validation | Size and MIME/magic-byte checks; images are fully decoded before publication, with a 16,384-pixel side cap, 36M pixels/image and 48M decoded pixels in flight. Remote downloads are bounded and staged to disk; uploads are capped in parallel. MP4/WebM are checked for supported container signatures but are not transcoded or malware-scanned. |
| Remote URL fetch | HTTP(S) only; all DNS answers must be public, the chosen IP is pinned for the connection, and every redirect is checked again (including through configured HTTP/HTTPS/SOCKS5 proxies). Request duration, size and redirects are limited. See limitations below. |
| Filesystem and metadata | Per-link locking serializes conflicting operations. Media and metadata updates use staging/rename and roll back on write errors. Malformed persisted metadata stops startup instead of getting discarded on a later save. Local-gallery access is confined to a configured directory. |
| PWA cache | The service worker caches only public application assets under `/static/`; admin pages/APIs and all mutable media links are network-only. It deletes previous `lanpaper-*` caches on activation. Static assets are revalidated when online; there is **no** claimed age/size-limited runtime cache. |

## Remaining limits and operator responsibilities

1. **Serve over HTTPS.** Basic Auth is sent with requests; HTTP exposes credentials. Lanpaper does not provision certificates, enforce HTTP→HTTPS redirects, manage sessions or supply an authorization proxy. Consider binding the backend to localhost/a private network if TLS terminates at a proxy.
2. **Trust the configured outbound proxy.** For plain HTTP targets, Lanpaper sends an absolute request URI containing the vetted IP and an original virtual-host header. RFC-compliant proxies route by the absolute URI; a proxy that instead routes by the Host header can defeat DNS pinning. Use a trusted, conformant proxy and keep it away from internal networks when possible. `INSECURE_SKIP_VERIFY=true` disables outbound TLS verification and must not be enabled in production. Any server with access to the public internet can still reach publicly routed addresses, including services inadvertently exposed there.
3. **Treat uploaded media as untrusted.** Decoders and browser media players may contain vulnerabilities. Keep the Go toolchain, WebP encoder dependency, runtime OS and browsers patched. Animated WebP is not supported; in compressed mode GIFs are reduced to one frame. Add scanning/sandboxing if administrators accept uploads from untrusted third parties.
4. **Back up `data/` and protect its permissions.** `data/wallpapers.json` includes link secrets. Staging/rename protects against ordinary failed operations but **does not guarantee durability under sudden power loss** (no explicit file/directory `fsync`); use persistent storage and periodic, tested backups. The JSON store is intended for a single server process sharing its own data directory, not multiple independent replicas.
5. **Handle migrations explicitly.** If upgrading from a version with media under `static/images/`, mount that old directory for the migration, then back up the new `data/` tree. Old cached media may remain in visitors' browsers until their old service worker activates the update or they clear site data. Changing server access levels cannot remotely revoke copies already saved by a browser.
6. **Set limits for your deployment.** The per-IP rate limiter is in memory; it is not a distributed DoS defense. `MAX_UPLOAD_MB` can be as high as 512 MiB, and decoded images/encoder work need more RAM than the compressed file size. Consider upstream request limits, a WAF or external rate limiting on public installations.

Run `go test -race ./...`, `go vet ./...`, `go mod verify` and `node --test tests/*.test.cjs` when changing security-sensitive code. For detailed audit results and deferred work, see [IMPROVEMENTS.md](IMPROVEMENTS.md).
