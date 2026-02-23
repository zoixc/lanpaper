# Lanpaper

A web-based wallpaper management service with support for image and video uploads.

## Features

- Upload images (JPEG, PNG, GIF, WebP, BMP, TIFF) and videos (MP4, WebM)
- Create short links for wallpaper access
- Automatic thumbnail generation
- Load images from URL or local server directory
- Basic Auth for admin panel protection (auto-disabled if no credentials provided)
- Enhanced security (CSP, path traversal protection)
- Rate limiting for abuse prevention
- Docker support
- Proxy support for external image downloads
- Modular code architecture

## What's New in v0.8.3

### Security Improvements
- Strict Content Security Policy (no `unsafe-inline` for scripts)
- Enhanced path traversal protection with absolute path validation
- Additional security headers (X-XSS-Protection, Permissions-Policy)
- File size validation before processing
- Logging of security violations

### Reliability
- Graceful shutdown with 30-second timeout
- HTTP server timeouts (Read: 30s, Write: 30s, Idle: 120s)
- Improved error handling with contextual logging

### Quality of Life
- Authentication automatically disabled if `ADMIN_USER` and `ADMIN_PASS` are not set
- No need to manually set `DISABLE_AUTH=true` for setups behind external auth

### Architecture
- Refactored into modules: `config`, `handlers`, `middleware`, `storage`, `utils`
- Reduced main.go from 900+ to 80 lines
- Better code readability and maintainability

See [CHANGELOG.md](CHANGELOG.md) for details

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
# Omit ADMIN_USER and ADMIN_PASS - auth will be auto-disabled
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

## Configuration

### Authentication Behavior

Authentication is automatically disabled if no credentials are provided:
- If `ADMIN_USER` and `ADMIN_PASS` environment variables are not set
- You will see a warning in logs: `"Warning: No credentials provided. Authentication is automatically disabled."`

This is useful when running behind external authentication (Tinyauth, Nginx Proxy Manager, Authelia, etc.)

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

## Project Structure

```
lanpaper/
├── main.go              # Entry point and routing
├── config/
│   └── config.go        # Application configuration
├── handlers/
│   ├── admin.go         # Admin API handlers
│   ├── upload.go        # Upload handlers
│   ├── public.go        # Public access
│   └── common.go        # Common utilities
├── middleware/
│   ├── auth.go          # Authentication
│   ├── security.go      # Security headers and CSP
│   └── ratelimit.go     # Rate limiting
├── storage/
│   └── wallpaper.go     # Data storage
└── utils/
    └── validation.go    # Path validation
```

## API Endpoints

### Public

- `GET /{linkName}` - Get image/video by short link (always public)

### Admin (requires Basic Auth if credentials are set)

- `GET /admin` - Admin panel
- `GET /api/wallpapers` - List all wallpapers
- `POST /api/link` - Create new link
  ```json
  {"linkName": "my-wallpaper"}
  ```
- `DELETE /api/link/{linkName}` - Delete link
- `POST /api/upload` - Upload image
  - Form data: `file` (file) or `url` (URL/path), `linkName`
- `GET /api/external-images` - List local images
- `GET /api/external-image-preview?path=...` - Preview local image

## Usage

### 1. Create Link

In admin panel, create a new link with name, e.g.: `sunset`

### 2. Upload Image

Upload an image for this link:
- Via file upload form
- By URL from internet
- From local server directory

### 3. Access

Image is now available at: `http://your-server:8080/sunset`

## Behind Reverse Proxy

### Example: Nginx Proxy Manager with External Auth

If you're using external authentication (Tinyauth, Nginx Proxy Manager, Authelia, etc.):

1. **Don't set `ADMIN_USER`/`ADMIN_PASS`** - auth will be auto-disabled
2. Configure your reverse proxy to protect `/admin` and `/api/*`

**Nginx example:**
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

## Automatic Cleanup

If `MAX_IMAGES` is set, old images are automatically deleted when the limit is exceeded (links are preserved).

## Security

- Content Security Policy without `unsafe-inline`
- X-Frame-Options: DENY
- X-Content-Type-Options: nosniff
- Path traversal protection
- Rate limiting
- Basic Authentication (optional)
- HTTP timeouts

### Production Recommendations

- Run behind a reverse proxy (nginx/Caddy/Traefik) with HTTPS
- Use external authentication (Tinyauth, Authelia) for better security
- Use strong passwords (minimum 16 characters)
- Regularly update Docker images
- Monitor logs for suspicious activity

## Docker Volumes

- `./data` - Wallpaper metadata (JSON)
- `./static/images` - Uploaded images and previews
- `./external/images` - External image directory (optional)

## Technologies

- Go 1.21+
- [golang.org/x/image](https://pkg.go.dev/golang.org/x/image) - Image resizing
- [github.com/chai2010/webp](https://github.com/chai2010/webp) - WebP support
- [github.com/joho/godotenv](https://github.com/joho/godotenv) - .env files

## Development

```bash
# Run
go run .

# Build
go build -o lanpaper .

# Docker build
docker build -t lanpaper .

# Run tests
go test ./...
```

## License

MIT License - see [LICENSE](LICENSE)

## Support

- Issues: [GitHub Issues](https://github.com/zoixc/lanpaper/issues)
- Changelog: [CHANGELOG.md](CHANGELOG.md)
