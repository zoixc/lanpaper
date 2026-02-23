# Lanpaper

> **One permanent link. Any content. Change it anytime.**

Create a static URL like `http://your-server/tv` and assign any image or video to it from the admin panel — without ever changing the address. Whatever device has that link embedded will always get the latest content you set.

**Perfect for:** digital photo frames, smart TVs, wallpaper engines, corporate displays, kiosks — anywhere the URL is fixed but the content needs to change.

```
http://your-server/bedroom   →  points to whatever image you set today
http://your-server/office    →  change it next week — the link stays the same
http://your-server/tv        →  swap video/image from the browser, no reconfiguration
```

![Lanpaper Admin Panel](docs/screenshot.png)

## Features

- **Permanent links with swappable content** — the core idea
- Upload images (JPEG, PNG, GIF, WebP, BMP, TIFF) and videos (MP4, WebM)
- Load content from URL or a local server directory
- Automatic thumbnail generation
- Basic Auth for admin panel (auto-disabled if no credentials set)
- Security: CSP, magic bytes validation, path traversal protection, rate limiting
- Docker with multi-arch images (amd64, arm64) — works on Raspberry Pi and TV boxes
- Proxy support for external image downloads
- i18n: EN, RU, DE, FR, IT, ES

## Quick Start

### Docker Compose (Recommended)

1. Copy example configuration:
```bash
cp docker-compose-example.yml docker-compose.yml
```

2. Edit `docker-compose.yml` and set your credentials.

3. Start:
```bash
docker-compose up -d
```

4. Open http://localhost:8080/admin

### Docker (Simple Run)

**With authentication:**
```bash
docker run -d \
  -p 8080:8080 \
  -e ADMIN_USER=admin \
  -e ADMIN_PASS=secret \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/static:/app/static \
  ptabi/lanpaper:latest
```

**Without authentication (behind external auth like Tinyauth/Authelia):**
```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/static:/app/static \
  ptabi/lanpaper:latest
```

### Local Build

```bash
go mod download
go build -o lanpaper .
./lanpaper
```

## How It Works

1. **Create a link** — give it a name, e.g. `bedroom`
2. **Assign content** — upload a file, paste a URL, or pick from server storage
3. **Use the link** — `http://your-server/bedroom` now serves that file directly
4. **Change anytime** — upload a new file to the same link from the admin panel; the URL never changes

## Configuration

### Authentication Behavior

Authentication is automatically disabled if credentials are not provided:
- Both `ADMIN_USER` and `ADMIN_PASS` must be set for auth to work
- If either is missing, auth is auto-disabled with a warning in logs

Useful when running behind external authentication (Tinyauth, Nginx Proxy Manager, Authelia, etc.)

### Via Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | Server port |
| `ADMIN_USER` | `` | Admin username (omit to disable auth) |
| `ADMIN_PASS` | `` | Admin password (omit to disable auth) |
| `DISABLE_AUTH` | `false` | Force-disable auth regardless of credentials |
| `MAX_UPLOAD_MB` | `50` | Max upload file size in MB |
| `MAX_IMAGES` | `0` | Max stored images (0 = unlimited) |
| `MAX_CONCURRENT_UPLOADS` | `2` | Max parallel uploads |
| `EXTERNAL_IMAGE_DIR` | `external/images` | Path to external image directory |
| `RATE_PUBLIC_PER_MIN` | `120` | Public endpoint rate limit (req/min) |
| `RATE_UPLOAD_PER_MIN` | `20` | Upload rate limit (req/min) |
| `RATE_BURST` | `10` | Rate limit burst size |
| `PROXY_TYPE` | `http` | Proxy type: `http`, `socks5` |
| `PROXY_HOST` | `` | Proxy host |
| `PROXY_PORT` | `` | Proxy port |
| `PROXY_USERNAME` | `` | Proxy username |
| `PROXY_PASSWORD` | `` | Proxy password |
| `INSECURE_SKIP_VERIFY` | `false` | Skip TLS verification for external requests |

### Via config.json

```json
{
  "port": "8080",
  "adminUser": "admin",
  "adminPass": "secret",
  "maxUploadMB": 50,
  "maxImages": 100,
  "maxConcurrentUploads": 2,
  "disableAuth": false,
  "externalImageDir": "external/images",
  "rate": {
    "publicPerMin": 120,
    "uploadPerMin": 20,
    "burst": 10
  },
  "proxyType": "http",
  "proxyHost": "",
  "proxyPort": "",
  "proxyUsername": "",
  "proxyPassword": "",
  "insecureSkipVerify": false
}
```

## API Endpoints

### Public

- `GET /{linkName}` — Serve image/video by link name (always public, no auth required)

### Admin (requires Basic Auth if credentials are set)

- `GET /admin` — Admin panel
- `GET /api/wallpapers` — List all links
- `POST /api/link` — Create new link `{"linkName": "my-wallpaper"}`
- `DELETE /api/link/{linkName}` — Delete link
- `POST /api/upload` — Upload content (form: `file` or `url`, `linkName`)
- `GET /api/external-images` — List files from server directory
- `GET /api/external-image-preview?path=...` — Preview server file
- `GET /health` — Health check (`status`, `version`, `uptime`)

## Behind Reverse Proxy

Recommended setup: run Lanpaper with no credentials and protect `/admin` + `/api/*` via your reverse proxy.

```nginx
location /admin {
    auth_request /auth;
    proxy_pass http://lanpaper:8080;
}

location /api/ {
    auth_request /auth;
    proxy_pass http://lanpaper:8080;
}

location / {
    proxy_pass http://lanpaper:8080;
}
```

## Security

- Content Security Policy (no `unsafe-inline`)
- Magic bytes validation for all uploaded files
- Path traversal protection
- Rate limiting per endpoint group
- X-Frame-Options, X-Content-Type-Options headers
- HTTP timeouts
- Atomic file writes (temp file + rename)

### Production Recommendations

- Run behind a reverse proxy (Nginx / Caddy / Traefik) with HTTPS
- Use external auth (Tinyauth, Authelia) for stronger protection
- Use strong passwords (minimum 16 characters)
- Mount `./data` and `./static/images` as Docker volumes

## Project Structure

```
lanpaper/
├── main.go              # Entry point and routing
├── config/              # Config loading and validation
├── handlers/            # HTTP handlers (admin, upload, public)
├── middleware/          # Auth, security headers, rate limiting
├── storage/             # In-memory store + atomic JSON persistence
└── utils/               # Validation helpers
```

## Docker Volumes

| Volume | Purpose |
|---|---|
| `./data` | Link metadata (JSON) |
| `./static/images` | Uploaded files and previews |
| `./external/images` | Optional: server-side image directory |

## Technologies

- Go 1.25+
- [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) — image processing
- [github.com/chai2010/webp](https://github.com/chai2010/webp) — WebP encoding
- [github.com/joho/godotenv](https://github.com/joho/godotenv) — `.env` support

## Development

```bash
go run .          # Run
go build -o lanpaper .   # Build
go test ./...     # Test
docker build -t lanpaper .  # Docker build
```

## Changelog

See [CHANGELOG.md](CHANGELOG.md).

## License

MIT — see [LICENSE](LICENSE).
