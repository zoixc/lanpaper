# Security policy and deployment notes

This page describes the controls that are **implemented**. It does not
guarantee that every hosting setup is safe. Run the current release, keep the
Go toolchain and base image patched, and serve the admin interface over
HTTPS.

Deployment, hardening and operations (topology, systemd/Docker, backups,
capacity, what to alert on) live in [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Reporting a vulnerability

If private vulnerability reporting is enabled for this repository, use
GitHub's **Report a vulnerability** / private security advisory. Otherwise,
contact the maintainers through a private channel listed on the repository
profile.

- Do not put exploit details in a public issue.
- Include the affected version, reproduction steps and impact.
- Do not include real credentials or access tokens.

## Trust boundaries

### Admin

The admin panel is protected by a sign-in form that sets an HttpOnly session
cookie (`SameSite=Lax`, 14 days, `Secure` over HTTPS). HTTP Basic Auth protects
`/api/*`, the private previews and admin-only links, and scripts use it.

- Sessions are stored as SHA-256 digests in `data/sessions.json` (mode 0600)
  and expire after 14 days. There is no password database: credentials come
  from the environment (or `config.json`) and must be handled as secrets.
- If either credential is missing, admin routes return **503**. They never
  fall back to anonymous access.
- `DISABLE_AUTH=true` is an explicit opt-out. Use it only if all three hold:
  - an independently authenticated reverse proxy protects the admin page,
  - that proxy also protects every `/api/*` route,
  - direct access to the Lanpaper backend is blocked.

### Brute-force protection

- **Lockout.** After 10 wrong username/password pairs from one client within
  15 minutes, the client is locked out until the window ends (`429`, with
  `Retry-After`).
- **No oracle during the lockout.** Credentials are not evaluated while a
  client is locked out, so even a correct guess is rejected.
- **Shared counter.** The admin API and admin-protected public links (`token`
  and `auth` levels) use the same counter, and so do wrong publish keys
  (`PUBLISH_KEYS`): guessing an API key costs the same budget as guessing a
  password, per client.
- **What counts as a client.** IPv6 clients are grouped by `/64`. Requests
  without credentials (the browser's first challenge) are not counted.
- **Behind a proxy**, configure `TRUSTED_PROXY`. Otherwise every visitor shares
  the proxy's address and can lock the others out.

### Publish keys

`PUBLISH_KEYS` authorizes *publishing* without the admin login. It is a
deliberately narrow credential:

| Route | With a valid publish key |
| --- | --- |
| `POST /api/upload` with `mode=append`, or with `autoCreate=1` for a link that does not exist yet | allowed |
| `POST /api/upload` with `mode=replace` (the default) on a link that already has media | `403` — only an admin login may replace a live file |
| `POST /api/link` | allowed |
| everything else under `/api/*` | falls back to Basic Auth, so a key alone gets `401` |
| `/admin` browser page | requires a sign-in session; Basic Auth is intentionally ignored so browser-cached credentials cannot defeat sign-out |

- **Environment only.** Keys are read from `PUBLISH_KEYS`, never from
  `config.json`, and the field is not serializable, so an exported or backed-up
  configuration cannot leak one.
- **Stored as digests.** Only SHA-256 digests are kept in memory and compared in
  constant time. Logs show an 8-hex-character fingerprint, never the key.
- **Parsing rules.** Entries shorter than 16 characters, duplicates and entries
  beyond the 32-key cap are dropped with a warning instead of being accepted.
- **No keys configured means no change.** When `PUBLISH_KEYS` is empty, a stray
  `X-Api-Key` header is ignored entirely: it is not a credential, cannot be
  guessed into a lockout, and cannot lock an administrator out.
- **A key is not a session.** It cannot read the link list, change access
  levels, roll back, rename or delete anything, and it does not bypass the CSRF
  check for browser requests, the rate limits, the upload validation or the
  SSRF rules for remote URLs.
- Rotate keys by changing the environment and restarting: there is no revocation
  list, and a leaked key can publish new content until it is replaced.

### Public links

| Level | Access rule |
| --- | --- |
| `public` | Anyone. |
| `local` | Decided by the client IP. |
| `token` | A generated 256-bit secret, or admin credentials. |
| `auth` | Admin Basic Auth. |

Previews of private links are available only through the admin API. With
built-in auth disabled, `auth`-level public URLs stay unavailable.

The access check always runs **before** a selector is resolved, so none of
`?v=` (archived version), `?i=` (playlist item) or the `.jpg` / `/latest`
aliases can bypass a link's level: an archived version of a `token` link needs
the same token as the live file. An unusable selector is a `404` rather than a
silent fallback to other bytes.

### Reverse proxy

Lanpaper uses the forwarded client IP, host and scheme **only** when
`TRUSTED_PROXY` matches the immediate peer.

- Set it narrowly: the proxy's IP or a small CIDR, never a client-accessible
  subnet. Several proxies — or a proxy plus the Docker bridge gateway the
  container actually sees — are listed comma-separated:
  `TRUSTED_PROXY="192.168.20.1,172.24.0.1"`. Only the listed addresses are
  believed; an entry that does not parse is dropped with a warning.
- The proxy must **append to or overwrite** `X-Forwarded-For`, and overwrite
  `X-Forwarded-Proto` and `X-Forwarded-Host`, so that no client-supplied value
  reaches Lanpaper unchanged. Lanpaper trusts only the rightmost
  `X-Forwarded-For` entry and never reads `X-Real-IP`: a proxy that does not
  overwrite a client-supplied `X-Real-IP` would otherwise let a client look like
  any address, including a private one.
- Without `TRUSTED_PROXY`, a proxy on a private network makes every visitor
  look `local`.

### Tokens

Token URLs are bearer secrets. They can end up in browser history and in
reverse-proxy access logs.

- Avoid logging query strings.
- `Referrer-Policy: no-referrer` stops browsers from leaking them in the
  Referer header.
- Rotate a leaked token; the old one stops working immediately.
- Browser exports never contain tokens. `data/wallpapers.json` does, so
  protect it.

## Implemented controls

| Area | Behaviour |
| --- | --- |
| Admin CSRF | State-changing browser requests are rejected when fetch metadata says `cross-site` or `same-site`, or when `Origin` (including its port) does not match. Non-browser clients must still authenticate. |
| Admin CSP | `default-src 'none'`. Scripts, styles, fonts, media, workers and fetches are same-origin only. Images also allow `data:` and `blob:` for the client-side compressor. `frame-ancestors 'none'`. No inline scripts or styles. |
| Headers | Everywhere: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`. Admin responses also get COOP `same-origin`, COEP `credentialless` and CORP `same-origin`. Public media allows cross-origin embedding and uses a sandboxing CSP. |
| CORS and embedding (opt-in) | By default public media sends no `Access-Control-Allow-Origin`, so a cross-origin `fetch()` or canvas read fails, and `X-Frame-Options: DENY` plus a `sandbox` CSP keep it unframable. `CORS_ORIGINS` adds `Access-Control-Allow-Origin` for listed origins only (parsed as URLs: `http`/`https`, no path, no credentials, no wildcard subdomains), with `Vary: Origin`, exposed read-only headers, `GET, HEAD, OPTIONS` and a filtered preflight header echo; unlisted origins get nothing and `OPTIONS` keeps answering `405`. `ALLOW_EMBED=true` drops `X-Frame-Options` and the CSP `sandbox` for `/{name}` only — `/admin` and `/api/*` stay unframable, and both flags are logged as warnings at startup. |
| Version history and playlists | Archived versions (`data/history/`) and playlist items (`data/items/`) are stored under validated link names and validated media extensions, in the same non-web-root tree as live media, and are reachable only through `/{name}` after the same access check. Both budgets are bounded (`HISTORY_LIMIT`, `HISTORY_MAX_MB`, `PLAYLIST_MAX`), trimming deletes the oldest archives first, and deleting or renaming a link moves or removes its directories with it. |
| Access counters | Per-link request, byte and last-hit counters live in memory only: no visitor identity, no IP, no user agent, no query string and nothing written to disk. They reset on restart and are visible to the authenticated admin only. |
| HTTP caching | The admin page and API responses are `no-store`. Admin previews and `public`/`local` media are `private, no-cache` with `ETag`/`Last-Modified`: browsers keep a private copy but revalidate it on every use, and shared caches must not store it. The access check runs before any `304`, so a revoked link is refused at once. `token` and `auth` media is `no-store`, so no copy outlives a token rotation on the client's disk. |
| Compression | Text responses are gzip-compressed; media never is. BREACH-style length oracles need attacker-chosen input reflected next to a secret in one compressed body. No response does that: JSON is rendered from stored state that only the admin can change, and query parameters only filter, sort or page through it. |
| HSTS | `Strict-Transport-Security: max-age=31536000` on responses to HTTPS requests, direct or forwarded by the trusted proxy. No `includeSubDomains` or `preload`. Plain-HTTP LAN use is unaffected. |
| Media isolation | Uploads live in `data/media/` and `data/previews/`, archived versions in `data/history/` and playlist items in `data/items/`. `/static/` serves an allowlist of application assets only; symlinks and swapped files are rejected. Media from legacy `static/images/` is migrated but never served statically. |
| Upload validation | Size limits, then content-based type detection (magic bytes). Images are fully decoded before publication. Limits: 16,384 px per side, 36 M pixels per image, 48 M decoded pixels in flight, and a limited number of parallel uploads. MP4/WebM are checked for container signatures only; they are not transcoded or scanned. |
| Remote URL fetch | HTTP(S) only. Every DNS answer must be a public unicast address. Private, loopback, link-local, CGNAT, documentation, multicast, NAT64, 6to4 and Teredo ranges are blocked. The vetted IP is pinned for the connection. Every redirect is checked again, including through HTTP, HTTPS and SOCKS5 proxies. Time, size and redirects are bounded. |
| Timeouts | Header read 10 s, request read 30 s, write 120 s, idle 120 s. Only authenticated uploads and preview regeneration extend their own deadlines. Shutdown waits up to 30 s for in-flight requests. |
| Filesystem and metadata | Per-link locks serialize conflicting operations, and metadata writers are queued. Readers never wait for disk I/O. Media and metadata are written to a temporary file, flushed with `fsync`, then atomically renamed. The metadata directory is synced as well. Failed writes roll back. Malformed metadata stops startup instead of being discarded on the next save. |
| Container | Non-root user (uid 100, gid 101). Application files are read-only for the service; only `data/` and the gallery directory are writable. The Compose example drops all capabilities and sets `no-new-privileges`. |
| PWA cache | The service worker caches only public application assets under `/static/` and fetches them from the network when online. Asset links from the admin page carry a content hash (`?v=`), so the browser caches those exact files for a year; any other URL revalidates. Admin pages, API responses and media links always go to the network. Old `lanpaper-*` caches are purged on activation. |
| Fault isolation | A panic in a handler is caught between the gzip layer and the router: it is logged with the method, the path and the stack, and answered with a plain `500` when the response has not started. No panic value, type, path or frame is ever sent to a client, and the query string is never logged because token links carry their secret there. `http.ErrAbortHandler` is re-raised untouched. The background prune worker and the rate-limit cleaner recover too, so neither can take the process down. |
| TLS (optional) | `TLS_CERT_FILE` + `TLS_KEY_FILE` terminate HTTPS in the process with `MinVersion` TLS 1.2; cipher suites and curves stay at Go's maintained defaults and HTTP/2 is negotiated automatically. Setting only one of the two is a fatal startup error rather than a silent fallback to plaintext. The usual deployment still terminates TLS at a reverse proxy with `TRUSTED_PROXY` set. |
| Crawler policy | `/robots.txt` disallows everything. Public links are mutable, credential-protected or both; indexing them costs bandwidth and publishes URLs whose access level may later be tightened. |
| Supply chain | Dependencies are verified by `go.sum`. CI runs `govulncheck`, pins GitHub Actions to commit SHAs and uses read-only `GITHUB_TOKEN` permissions. Dependabot proposes updates. Every first-party source file carries an `SPDX-License-Identifier: MIT` header and all bundled components are listed with their licences in [THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md) (permissive only — no copyleft). |

## Operator responsibilities and limits

1. **Serve over HTTPS.** The sign-in form and Basic Auth both send the password,
   so plain HTTP exposes it on the network. Terminate TLS at a reverse proxy
   (recommended, and the only setup that also gives you an HTTP→HTTPS
   redirect), or set `TLS_CERT_FILE` + `TLS_KEY_FILE` to let Lanpaper serve
   HTTPS itself. Lanpaper never redirects HTTP to HTTPS on its own and provides
   no authorization proxy, so bind the listener to localhost or a private
   network whenever something else terminates TLS.
2. **Trust your outbound proxy.** For plain-HTTP targets, Lanpaper sends an
   absolute request URI with the vetted IP, plus the original `Host` header.
   A proxy that routes by `Host` instead can defeat DNS pinning.
   `INSECURE_SKIP_VERIFY=true` disables outbound TLS verification; do not
   enable it in production.
3. **Treat uploaded media as untrusted.** Decoders and media players can have
   bugs. Keep Lanpaper, the base image and browsers updated. Add scanning or
   sandboxing if people you don't trust can upload.
4. **Back up `data/` and protect its permissions.** `data/wallpapers.json`
   contains link tokens. The JSON store is meant for one server process that
   owns its data directory, not for several replicas.
5. **Set limits for your deployment.** The rate limiters and the login lockout
   live in memory and reset on restart. They are not a distributed DoS
   defence. `MAX_UPLOAD_MB` can go up to 512 MiB, and decoding needs more RAM
   than the file size. Consider upstream request limits on public
   installations.
6. **Migrations and caches.** Changing an access level cannot remove copies
   that a client has already downloaded. Rolling a link back restores older
   bytes at the same URL, but clients that already cached the newer file keep it
   until they revalidate.
7. **Publish keys are write credentials.** Give them only to automation that
   needs to push content, keep them out of URLs, logs and version control, and
   replace them by restarting with a new `PUBLISH_KEYS` when one leaks. A key can
   add media to any link and create new links, but it cannot replace the live
   file of a link that already has media: that remains an admin action, so a
   leaked key cannot deface links that are in use.
8. **CORS and embedding widen who can read public media.** `CORS_ORIGINS: *`
   lets any website read every `public` link with JavaScript, and
   `ALLOW_EMBED=true` lets any website frame it. List only the origins you
   control, and remember that `token`, `local` and `auth` links still require
   their own credential.
9. **Budget the disk.** Version history keeps up to `HISTORY_LIMIT` extra copies
   per link within `HISTORY_MAX_MB` for the whole server, and a playlist can
   hold up to `PLAYLIST_MAX` extra files per link. Set `HISTORY_LIMIT=0` to
   store only the live file, as before.

## Reverse proxy examples

Set `TRUSTED_PROXY` to the address the proxy connects from, for example
`127.0.0.1` or the Docker network gateway — several addresses are listed
comma-separated (`192.168.20.1,172.24.0.1`). This makes rate limiting, the
login lockout and the `local` access level see the real client IP. Do **not**
set it when Lanpaper is exposed directly.

If the panel answers `403 Cross-origin request rejected`, the log line names
the address to trust: it prints `RemoteAddr` and the `Host` /
`X-Forwarded-Host` the proxy sent. Behind a proxy the `Host` must be the name
the browser used — a proxy that rewrites `Host` (or sends it with a scheme)
fails the check even with `TRUSTED_PROXY` set.

### nginx

```nginx
server {
    listen 443 ssl;
    server_name lanpaper.example.com;

    ssl_certificate     /etc/ssl/certs/lanpaper.crt;
    ssl_certificate_key /etc/ssl/private/lanpaper.key;

    client_max_body_size 210m;   # at least MAX_UPLOAD_MB + 1 MB

    location / {
        proxy_pass         http://127.0.0.1:8080;
        # The original Host, scheme included only in $scheme: the CSRF check
        # compares Origin against this name, so it must be the one the browser
        # used. $http_host keeps the port and the exact spelling.
        proxy_set_header   Host              $http_host;
        proxy_set_header   X-Forwarded-For   $remote_addr;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_set_header   X-Forwarded-Host  $http_host;
        proxy_request_buffering off;
        proxy_read_timeout 600s;
    }
}
```

### Caddy (automatic HTTPS)

```caddyfile
lanpaper.example.com {
    reverse_proxy 127.0.0.1:8080
}
```

Caddy sets `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host`
itself and preserves `Host`. With any other proxy, make sure that
`X-Forwarded-For` is appended to or overwritten by the proxy, never passed
through from the client.

## Checks for contributors

See [CONTRIBUTING.md](CONTRIBUTING.md) for the invariants a change must not
break. Run these before changing security-sensitive code:

```sh
gofmt -l . && go vet ./... && go test -race ./...
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
node --test tests/*.test.cjs
```
