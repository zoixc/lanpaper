# Security Guide

## Credentials

Credentials are loaded from **environment variables** (or a `.env` file). Never store secrets in `config.json`.

```bash
# .env (chmod 600, never commit to git)
ADMIN_USER=admin
ADMIN_PASS=your-strong-password
```

> **Important:** Without credentials, authentication is disabled automatically. Always set both `ADMIN_USER` and `ADMIN_PASS` in any non-isolated environment.

## Per-link access levels

Each wallpaper link has an `accessLevel`:

| Level | Who can open `/{linkName}` |
|-------|----------------------------|
| `public` (default) | Anyone on the internet |
| `local` | Clients whose IP is loopback, RFC1918, link-local, CGNAT, or IPv6 ULA |
| `token` | Requests with `?token=` or `X-Access-Token` matching the link secret (admin Basic Auth also allowed) |
| `auth` | Valid admin Basic Auth only |

Media files are stored under `data/media/` and `data/previews/` — **outside** the static web root. Direct URLs like `/static/images/...` are blocked so access control cannot be bypassed.

Admin thumbnails are served only via `/api/preview/{name}` (behind admin auth).

## HTTPS / TLS

Lanpaper does not terminate TLS itself. **Always place it behind a reverse proxy** that handles TLS:

### Nginx example

```nginx
server {
    listen 443 ssl;
    server_name lanpaper.yourdomain.local;

    ssl_certificate     /etc/ssl/certs/lanpaper.crt;
    ssl_certificate_key /etc/ssl/private/lanpaper.key;
    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         HIGH:!aNULL:!MD5;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_set_header   Host $host;
        proxy_set_header   X-Real-IP $remote_addr;
        proxy_set_header   X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;
    }
}
```

Set `TRUSTED_PROXY` to the proxy's address (or CIDR) so rate limiting and
the `local` access level see the real client IP. **Do not** set it when
Lanpaper is exposed directly — forged `X-Forwarded-*` headers would otherwise
be trusted.

### Caddy example (automatic HTTPS)

```
lanpaper.yourdomain.local {
    reverse_proxy localhost:8080
}
```

## SSRF Protection

When users upload images via URL, the server fetches the remote file. Lanpaper blocks all requests to private or reserved IP ranges:

- `127.0.0.0/8` — loopback
- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` — RFC 1918 private networks
- `169.254.0.0/16` — link-local / AWS Instance Metadata Service
- `100.64.0.0/10` — CGNAT
- IPv6 loopback, ULA, and link-local ranges

Protection is applied at **three layers**:

1. Pre-request DNS / IP check (`ValidateRemoteURL`) — required when an HTTP proxy is configured, because the dialer only sees the proxy address
2. At TCP dial time via `ssrfSafeDialer` (prevents DNS rebinding)
3. On every redirect hop (`CheckRedirect` + `ValidateRemoteURL`, max 5 redirects)

## CSRF

State-changing admin requests require same-origin signals (`Sec-Fetch-Site` or matching `Origin`). `X-Forwarded-Host` is only consulted when the peer is the configured `TRUSTED_PROXY`.

## Docker

The container runs as a **non-root user** (`lanpaper`). No special action needed — it's the default.

Avoid mounting sensitive host directories into the container. The only directories that need to be mounted are:

```yaml
volumes:
  - ./data:/app/data
  - ./external/images:/app/external/images   # optional gallery
```

Do **not** bind-mount host media into `/app/static/images` for serving — that path is intentionally not exposed over HTTP.
