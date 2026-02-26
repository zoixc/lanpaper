# Security Guide

## Credentials

Credentials are loaded from **environment variables** (or a `.env` file). Never store secrets in `config.json`.

```bash
# .env (chmod 600, never commit to git)
ADMIN_USER=admin
ADMIN_PASS=your-strong-password
```

> **Important:** Without credentials, authentication is disabled automatically. Always set both `ADMIN_USER` and `ADMIN_PASS` in any non-isolated environment.

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

Protection is applied at **two layers**:
1. Pre-request DNS check (`ValidateRemoteURL`)
2. At TCP dial time via `ssrfSafeDialer` (prevents DNS rebinding attacks)

## Docker

The container runs as a **non-root user** (`lanpaper`). No special action needed — it's the default.

Avoid mounting sensitive host directories into the container. The only directories that need to be mounted are:

```yaml
volumes:
  - ./data:/app/data
  - ./external/images:/app/external/images
```
