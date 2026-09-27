# Security and performance audit

This page records the work on the current branch, not historical claims about another branch. For configuration, installation and API behavior see [README.md](README.md); for the deployment threat model see [SECURITY.md](SECURITY.md).

## Completed

| Area | Change | Why it matters |
| --- | --- | --- |
| Authentication/access | Missing admin credentials now fail closed with HTTP 503 instead of silently disabling Basic Auth; `DISABLE_AUTH=true` remains an explicit opt-out. Admin previews require auth, and `/static/` serves only allowlisted application assets, not media. Public-link `local`/`token`/`auth` rules are tested. | Prevents anonymous administration and bypassing link ACLs via old media paths or thumbnails. |
| Browser isolation | Added same-origin checks for unsafe methods, corrected security headers/CSP for actual assets, and set `no-referrer` and `no-store` where secrets/private responses are involved. | Limits CSRF, script injection, framing and token leakage without breaking images embedded on external displays. |
| Outbound requests | Validates every DNS answer and redirect, pins the vetted IP even when a proxy is configured, preserves original TLS SNI/virtual host, fixes HTTP proxy absolute-form request-target pinning, and bounds download size/time/redirects. | Reduces SSRF and DNS-rebinding risk and avoids buffering large downloaded files in RAM. A nonconforming outbound proxy remains an operational risk. |
| Upload processing | Validates format bytes and full image decoding; bounds image dimensions, pixel count, total decoded pixels and parallel uploads. Streams downloads and staged media; uses the streaming pure-Go WebP **decoder** rather than the dependency's whole-file-buffering C decoder (C encoder remains). Invalid images cannot be published in lossless mode. | Bounds common decompression bombs and memory pressure while preserving still-WebP and other supported formats. |
| Media/metadata consistency | Added per-link locks, stage-and-rename publishing with rollback, and copy-on-write JSON store commits. Startup refuses malformed metadata. Link rename, prune, delete, upload and preview regeneration recheck state; symlinks to private files are not served. | A failed disk write no longer reports success or leaves an in-memory ACL that disagrees with disk; concurrent operations cannot silently reintroduce stale media. |
| Browser code | JPEG-only pre-upload optimization sends the original if recompression grows it; preserves PNG transparency and GIF format. Hardened imported backups (size, link count and name validation), uses text nodes for dynamic content and does not overwrite existing media on import. | Preserves formats and reduces needless work/XSS exposure. Exports remain link manifests, **not** full backups. |
| Service worker | Root-scoped registration; network-first **static assets only**; rejects private/no-store responses; purges earlier runtime/admin/media caches. | Cached public images or old admin responses cannot bypass later access-level changes through the updated worker. |
| Maintenance | Reduced duplicate link/validation/transport code, added cached sorted snapshots, preserved optional API pagination, bounded the external directory walk, updated Go image dependencies and Docker builder/runtime base, replaced sample default passwords with a fail-closed configuration. | Improves maintainability, scaling and deployability. |

### Compatibility notes

- Public links, named slots, uploads (local/remote/server directory), access levels, search, pinning, pagination, import/export and six UI languages remain available. `/admin.html` redirects to `/admin`; public media remains embeddable cross-origin.
- The default is **not** unauthenticated administration. Deployments previously relying on omitted credentials must configure both `ADMIN_USER` and `ADMIN_PASS`, or *intentionally* set `DISABLE_AUTH=true` behind an external authentication proxy. A Compose example now requires an explicitly supplied password.
- Media is stored in `data/media` and previews in `data/previews`; old `static/images` media is migrated if accessible. Back up `data/` before upgrading and mount legacy media for the initial migration.
- Lossless mode (`quality=100`, `scale=100`) validates images and preserves original bytes for supported formats, including BMP/TIFF. Other settings may re-encode; GIF animation survives only when the original GIF is preserved. Animated WebP was unsupported by the prior and current decoders and is still rejected rather than silently flattened.
- With no credentials, `auth`-level public links remain unavailable even if an external proxy authenticates admin routes. The protected admin preview route remains usable behind that proxy.

## Verification

- `go test ./... -count=1` and `go test -race ./... -count=1 -timeout=120s`: all packages pass, including HTTP routing, access controls, upload rollback, proxy request-target pinning (HTTP/HTTPS/SOCKS5), pixel budgets, concurrency, corrupt-metadata startup and store atomicity.
- `go vet ./...` and `go mod verify`: pass; module sums checked against Go's checksum database.
- `node --test tests/*.test.cjs`: service-worker cache isolation and client-side compression tests pass. JavaScript syntax and i18n JSON were validated.
- Browser smoke test in Chromium: admin authentication, create/delete, PNG upload and thumbnail, public link, token control, language selection, service worker registration and mobile-width layout passed. (Browser dependencies/test runner were in a temporary environment, not required by the Go test suite.)
- **Not run here:** Docker image/multi-architecture builds (no Docker daemon in the test environment), external-proxy deployments against real remote proxy services, or long-duration/load tests.

## Recommended follow-ups

| Priority | Improvement | Reason / trade-off |
| --- | --- | --- |
| High | Add durability-focused storage: `fsync` staged data and parent directory or migrate to a transactional single-writer database; test power-loss recovery and multi-instance behavior. | Renames protect against ordinary failures, not sudden power loss; the current JSON store is single-process. |
| High | Consider an authenticated CONNECT tunnel for outbound HTTP proxies and add proxy-conformance integration tests. | Plain HTTP uses a pinned absolute URI plus an original Host header; proxies that ignore the request-target can still re-resolve a hostile name. Requiring CONNECT may break proxies that disallow tunneling to port 80; make that compatibility decision explicitly. |
| Medium | Add optional animated WebP support and animation-preserving GIF/WebP transforms. | Current image decoders cannot decode animated WebP; compressed GIF is flattened to its first frame. Requires frame-count, pixel and CPU budgets. |
| Medium | Implement a real encrypted, media-inclusive backup/restore path, with explicit overwrite/conflict semantics and token handling. | Browser JSON import/export currently handles link names/preferences, not media or full link access settings. |
| Medium | Paginate the admin UI and external gallery listings for very large deployments; add measurements before further buffer changes. | The API already supports pagination but the default UI loads all links. |
| Medium | Add CI Docker builds (amd64 and arm64), HTTP/SOCKS proxy e2e tests, browser integration coverage for import/rename/local access, and fuzz/property tests for media/URL validators. | Unit/race/smoke checks do not exercise every deployment path or decoder edge case. |
| Operational | Add external TLS termination, monitoring, upstream request/rate limits and optional media scanning/sandboxed decoding. | These controls depend on deployment and threat model; they are not built into the application. |

No change should be considered a complete security guarantee. Review the trust and operational limits in [SECURITY.md](SECURITY.md) before public deployment.
