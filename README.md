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

## What's New in v0.8.0

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
cp config.example.json config.json
```

2. Edit `config.json` and set credentials:
```json
{
  "port": "8080",
  "username": "admin",
  "password": "your_secure_password",
  "maxUploadMB": 50
}
```

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

**Without authentication (behind external auth like Nginx):**
```bash
# Just omit ADMIN_USER and ADMIN_PASS - auth will be auto-disabled
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

**Important**: Authentication is automatically disabled if no credentials are provided:
- If `ADMIN_USER` and `ADMIN_PASS` environment variables are not set
- OR if `username` and `password` are empty in `config.json`
- You will see a warning in logs: `"Warning: No credentials provided. Authentication is automatically disabled."`

This is useful when running behind external authentication (Nginx Proxy Manager, Authelia, etc.)

### Via config.json

```json
{
  "port": "8080",
  "username": "admin",
  "password": "secret",
  "maxUploadMB": 50,
  "maxImages": 100,
  "max_concurrent_uploads": 3,
  "disableAuth": false,
  "externalImageDir": "external/images",
  "rate": {
    "public_per_min": 50,
    "admin_per_min": 0,
    "upload_per_min": 20,
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

```bash
export PORT=8080
export ADMIN_USER=admin           # Optional - auth disabled if not set
export ADMIN_PASS=secret          # Optional - auth disabled if not set
export MAX_UPLOAD_MB=50
export MAX_IMAGES=100
export DISABLE_AUTH=false         # Optional - auto-set if no credentials
export RATE_LIMIT=50
export EXTERNAL_IMAGE_DIR=/path/to/images

# Proxy settings
export PROXY_TYPE=http
export PROXY_HOST=proxy.example.com
export PROXY_PORT=8080
export PROXY_USER=username
export PROXY_PASS=password
export INSECURE_SKIP_VERIFY=false
```

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

If you're using external authentication (Nginx Proxy Manager, Authelia, etc.):

1. **Don't set `ADMIN_USER`/`ADMIN_PASS`** - auth will be auto-disabled
2. Configure your reverse proxy to:
   - Protect `/admin` and `/api/*` with external auth
   - Allow public access to `/{linkName}` (direct image links)

**Nginx example:**
```nginx
location /admin {
    auth_request /auth;  # Your external auth
    proxy_pass http://lanpaper:8080;
}

location /api/ {
    auth_request /auth;  # Your external auth
    proxy_pass http://lanpaper:8080;
}

location / {
    # Public access for direct image links
    proxy_pass http://lanpaper:8080;
}
```

This way:
- Admin panel requires external authentication
- Direct image links (e.g., `/sunset`) work without auth
- No need to manage Basic Auth credentials in Lanpaper

## Automatic Cleanup

If `maxImages` is set, old images are automatically deleted when limit is exceeded (links are preserved).

## Security

### Implemented Protection

- Content Security Policy without `unsafe-inline`
- X-Frame-Options: DENY
- X-Content-Type-Options: nosniff
- X-XSS-Protection
- Path traversal protection
- All user paths validated
- Rate limiting
- Basic Authentication (optional)
- HTTP timeouts

### Production Recommendations

**Important**: Use HTTPS in production! Recommended:
- Run behind reverse proxy (nginx/Caddy/Traefik)
- Configure TLS certificates
- Use external authentication system for better security
- If using built-in auth, use strong passwords (minimum 16 characters)
- Configure rate limiting for your load
- Regularly update Docker images
- Monitor logs for suspicious activity

## Docker Volumes

- `./data` - Wallpaper metadata (JSON)
- `./static/images` - Uploaded images and previews
- `./external/images` - External image directory (optional)

## Technologies

- Go 1.25+
- [github.com/nfnt/resize](https://github.com/nfnt/resize) - Image resizing
- [github.com/chai2010/webp](https://github.com/chai2010/webp) - WebP support
- [github.com/joho/godotenv](https://github.com/joho/godotenv) - .env files

## Development

```bash
# Run in dev mode
go run main.go

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
- Discussions: [GitHub Discussions](https://github.com/zoixc/lanpaper/discussions)
- Changelog: [CHANGELOG.md](CHANGELOG.md)

## Roadmap

- [ ] Unit tests
- [ ] Integration tests
- [ ] GitHub Actions CI/CD
- [ ] Built-in TLS support
- [ ] S3/cloud storage support
- [ ] Bulk API operations
- [ ] Wallpaper search
- [ ] Tags and categories
- [ ] Per-user API rate limiting
- [ ] Metrics and Prometheus support
