# Security policy and deployment notes

This page describes the controls that are **implemented**. It does not
guarantee that every hosting setup is safe. Run the current release, keep the
Go toolchain and base image patched, and serve the admin interface over
HTTPS.

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

HTTP Basic Auth protects `/admin`, `/api/*` and the private previews.

- There are no sessions and no password database. Credentials come from the
  environment (or `config.json`) and must be handled as secrets.
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
  and `auth` levels) use the same counter.
- **What counts as a client.** IPv6 clients are grouped by `/64`. Requests
  without credentials (the browser's first challenge) are not counted.
- **Behind a proxy**, configure `TRUSTED_PROXY`. Otherwise every visitor shares
  the proxy's address and can lock the others out.

### Public links

| Level | Access rule |
| --- | --- |
| `public` | Anyone. |
| `local` | Decided by the client IP. |
| `token` | A generated 256-bit secret, or admin credentials. |
| `auth` | Admin Basic Auth. |

Previews of private links are available only through the admin API. With
built-in auth disabled, `auth`-level public URLs stay unavailable.

### Reverse proxy

Lanpaper uses the forwarded client IP, host and scheme **only** when
`TRUSTED_PROXY` matches the immediate peer.

- Set it narrowly: the proxy's IP or a small CIDR, never a client-accessible
  subnet.
- The proxy must **replace** client-supplied `X-Real-IP` / `X-Forwarded-*`
  headers, not pass them through.
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
| Headers | Everywhere: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`. Admin responses also get COOP `same-origin`, COEP `credentialless`, CORP `same-origin` and `Cache-Control: no-store`. Public media allows cross-origin embedding, uses `no-store` and a sandboxing CSP. |
| HSTS | `Strict-Transport-Security: max-age=31536000` on responses to HTTPS requests, direct or forwarded by the trusted proxy. No `includeSubDomains` or `preload`. Plain-HTTP LAN use is unaffected. |
| Media isolation | Uploads live in `data/media/` and `data/previews/`. `/static/` serves an allowlist of application assets only; symlinks and swapped files are rejected. Media from legacy `static/images/` is migrated but never served statically. |
| Upload validation | Size limits, then content-based type detection (magic bytes). Images are fully decoded before publication. Limits: 16,384 px per side, 36 M pixels per image, 48 M decoded pixels in flight, and a limited number of parallel uploads. MP4/WebM are checked for container signatures only; they are not transcoded or scanned. |
| Remote URL fetch | HTTP(S) only. Every DNS answer must be a public unicast address. Private, loopback, link-local, CGNAT, documentation, multicast, NAT64, 6to4 and Teredo ranges are blocked. The vetted IP is pinned for the connection. Every redirect is checked again, including through HTTP, HTTPS and SOCKS5 proxies. Time, size and redirects are bounded. |
| Timeouts | Header read 10 s, request read 30 s, write 120 s, idle 120 s. Only authenticated uploads and preview regeneration extend their own deadlines. Shutdown waits up to 30 s for in-flight requests. |
| Filesystem and metadata | Per-link locks serialize conflicting operations. Media and metadata are written to a temporary file, flushed with `fsync`, then atomically renamed. The metadata directory is synced as well. Failed writes roll back. Malformed metadata stops startup instead of being discarded on the next save. |
| Container | Non-root user (uid 100, gid 101). Application files are read-only for the service; only `data/` and the gallery directory are writable. The Compose example drops all capabilities and sets `no-new-privileges`. |
| PWA cache | The service worker caches only public application assets under `/static/` and revalidates them when online. Admin pages, API responses and media links always go to the network. Old `lanpaper-*` caches are purged on activation. |
| Supply chain | Dependencies are verified by `go.sum`. CI runs `govulncheck`, pins GitHub Actions to commit SHAs and uses read-only `GITHUB_TOKEN` permissions. Dependabot proposes updates. |

## Operator responsibilities and limits

1. **Serve over HTTPS.** Basic Auth sends the password with every request, so
   plain HTTP exposes it on the network. Lanpaper does not terminate TLS. It
   also does not redirect HTTP to HTTPS or provide an authorization proxy.
   Bind the backend to localhost or a private network when a proxy
   terminates TLS.
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
   that a client has already downloaded.

## Reverse proxy examples

Set `TRUSTED_PROXY` to the proxy address, for example `127.0.0.1` or the
Docker network gateway. This makes rate limiting, the login lockout and the
`local` access level see the real client IP. Do **not** set it when Lanpaper
is exposed directly.

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
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $remote_addr;
        proxy_set_header   X-Forwarded-Proto $scheme;
        proxy_set_header   X-Forwarded-Host  $host;
        proxy_request_buffering off;
        proxy_read_timeout 600s;
    }
}
```

### Caddy (automatic HTTPS)

```caddyfile
lanpaper.example.com {
    reverse_proxy 127.0.0.1:8080 {
        # Caddy passes a client-supplied X-Real-IP through unchanged, and
        # Lanpaper prefers X-Real-IP over X-Forwarded-For. Always overwrite it.
        header_up X-Real-IP {remote_host}
    }
}
```

Caddy sets `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host`
itself and preserves `Host`. With any other proxy, make sure that both
`X-Real-IP` and `X-Forwarded-For` are set by the proxy, never passed through
from the client.

## Checks for contributors

Run these before changing security-sensitive code:

```sh
gofmt -l . && go vet ./... && go test -race ./...
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
node --test tests/*.test.cjs
```
