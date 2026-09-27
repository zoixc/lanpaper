# Lanpaper

> **One permanent link. Any content. Change it anytime.**

Create a link such as `https://your-server/tv`, assign an image or video in the admin panel, and change its content without changing the link. Useful for digital frames, smart TVs, kiosks and displays.

## Features

- Stable named links with swappable JPEG, PNG, GIF, WebP, BMP, TIFF, MP4 or WebM content.
- Upload a file, fetch a public HTTP(S) URL, or choose from a configured server-side directory.
- Thumbnail generation, optional image compression/scaling and an original-byte preservation mode.
- Per-link `public`, `local`, `token` and `auth` access levels; media and previews are outside the static web root.
- Admin panel with Basic Auth, search, sorting, pinning, import/export of link names, dark mode and six languages (EN, RU, DE, FR, IT, ES).
- Docker deployment, outbound HTTP/HTTPS/SOCKS5 proxy support and rate limits.

## Quick start

### Docker Compose

```sh
cp docker-compose-example.yml docker-compose.yml
printf 'ADMIN_PASS=%s\n' "$(openssl rand -hex 24)" > .env
chmod 600 .env
docker compose up -d
```

Open <http://localhost:8080/admin> and log in as `admin` using the generated password in `.env`. The example publishes port 8080 on all interfaces: use HTTPS at your reverse proxy before exposing it to the internet. To expose only to a local reverse proxy, change the Compose port binding to `127.0.0.1:8080:8080`. Keep `.env` private; it is ignored by Git.

### Docker run

```sh
: "${ADMIN_PASS:?Set a unique, long ADMIN_PASS in your shell first}"
docker run -d -p 8080:8080 \
  -e ADMIN_USER=admin -e ADMIN_PASS="$ADMIN_PASS" \
  -v "$(pwd)/data:/app/data" ptabi/lanpaper:latest
```

### Build locally

Requires Go 1.25.3+ and a C compiler (for WebP **encoding**). Go 1.26 is used for the Docker build.

```sh
go mod download
go test ./...
go build -o lanpaper .
ADMIN_USER=admin ADMIN_PASS='set-a-unique-long-password' ./lanpaper
```

If credentials are missing, **admin routes return HTTP 503**; they are *not* made public. To use an external authentication proxy instead, explicitly set `DISABLE_AUTH=true`, restrict direct access to the backend and enforce authentication on `/admin` **and** `/api/*` at that proxy. Public links and `/health` remain accessible without admin credentials.

## Usage and access control

1. Create a name, e.g. `bedroom`, in `/admin`.
2. Upload content, enter a public URL, or choose a file from `EXTERNAL_IMAGE_DIR`.
3. Open `https://your-server/bedroom` on the display. Subsequent uploads keep the same URL.

Link names must start with an ASCII letter or digit and contain only letters, digits, `_` or `-` (maximum 64 characters). A newly created link has no media until the first upload.

| Access level | Who can open `GET /{name}` |
| --- | --- |
| `public` (default) | Anyone who knows the URL |
| `local` | Clients on loopback, private/LAN, link-local or CGNAT networks |
| `token` | A valid `?token=` or `X-Access-Token` header; admin Basic Auth also works |
| `auth` | Admin Basic Auth only |

A token is generated on selection and can be rotated from the UI or API; the previous token immediately stops working. Token URLs are secrets: avoid sharing them in logs or analytics. The admin preview endpoint is always admin-protected. With `DISABLE_AUTH=true`, an `auth`-level public link is not available via `/{name}` (the authenticated external proxy can still serve the admin preview).

**Behind a reverse proxy:** Set `TRUSTED_PROXY` to *only* the proxy's IP/CIDR, and configure that proxy to overwrite incoming `X-Real-IP`, `X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host`. Otherwise a `local` link can appear local to *every* visitor because the proxy itself is on a private network. Preserve the original Host/scheme for same-origin admin requests. Do not trust an entire client-accessible subnet.

## Configuration

Defaults are overridden by optional `config.json`, then environment variables. See [config.example.json](config.example.json) for JSON field names. Never leave its empty `adminPass` unchanged if you need the admin panel. Changes require a restart.

| Environment variable | Default | Notes |
| --- | --- | --- |
| `PORT` | `8080` | Listening port |
| `ADMIN_USER`, `ADMIN_PASS` | unset | Both required for Basic Auth; missing credentials deny admin access |
| `DISABLE_AUTH` | `false` | Explicit, potentially dangerous opt-out for an external auth proxy |
| `MAX_UPLOAD_MB` | `50` | Per file, 1–512 MiB; request/download limits also apply |
| `MAX_IMAGES` | `0` | `0` = unlimited; prunes the oldest non-pinned media when set |
| `MAX_CONCURRENT_UPLOADS` | `2` | Concurrent upload limit, 1–8 |
| `MAX_WALK_DEPTH` | `3` | External directory recursion limit, 1–10 |
| `EXTERNAL_IMAGE_DIR` | `external/images` | Directory for server-side imports; mount read-only if possible |
| `RATE_PUBLIC_PER_MIN` | `120` | Public link requests per client/minute |
| `RATE_UPLOAD_PER_MIN` | `20` | Upload and regeneration request limit per client/minute |
| `RATE_BURST` | `10` | Additional requests permitted per window |
| `COMPRESSION_QUALITY` | `85` | JPEG/WebP quality, 1–100 |
| `COMPRESSION_SCALE` | `100` | Image size percentage, 1–100 |
| `PROXY_TYPE` | `http` | Outbound `http`, `https` or `socks5` |
| `PROXY_HOST`, `PROXY_PORT` | unset | Optional outbound proxy |
| `PROXY_USERNAME`, `PROXY_PASSWORD` | unset | Proxy credentials |
| `INSECURE_SKIP_VERIFY` | `false` | Skips outbound TLS verification; development only |
| `TRUSTED_PROXY` | unset | Incoming reverse proxy IP or CIDR; forwarded headers trusted only from it |

### Image formats and compression

- At `COMPRESSION_QUALITY=100` **and** `COMPRESSION_SCALE=100`, supported image files are fully decoded/validated, then their original bytes are saved unchanged. This preserves format and animation where the decoder accepts it. It does **not** skip validation or preview generation. BMP/TIFF files are also preserved in this mode.
- At other values, the server decodes, optionally scales and re-encodes. BMP/TIFF become JPEG; a re-encoded GIF contains its first frame. MP4/WebM are copied without image re-encoding.
- The browser pre-processes only JPEGs, and sends the original if the transformed file is larger; PNG, GIF, WebP and other formats are left to the server. Animated WebP is **not currently supported** by the image decoders; static WebP works.
- Images are capped at 16,384 pixels per dimension, 36 million pixels per image and 48 million decoded pixels in flight. Large or malformed media may be rejected even below `MAX_UPLOAD_MB`.

### Remote URLs

Only HTTP(S) URLs resolving exclusively to public IPs are permitted. Each redirect is checked again; connections use a vetted IP to reduce DNS-rebinding risk, including through a configured proxy. A configured **HTTP proxy must respect the IP authority in the absolute request URI**, rather than re-resolving the separate virtual-host header. Use a trusted proxy and keep its access controls up to date. Downloads are streamed to bounded temporary files; local-gallery URLs never leave `EXTERNAL_IMAGE_DIR`.

## API

Admin endpoints require Basic Auth unless `DISABLE_AUTH=true` (which requires external protection). State-changing browser requests also require a matching origin.

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/admin` | Admin panel |
| `GET` | `/api/wallpapers` | All links; optional `page=1&page_size=50` (max 200), filters/sort |
| `POST` | `/api/link` | Create a link (`{"linkName":"bedroom"}`) |
| `PATCH` | `/api/link/{name}` | Rename (`newLinkName`), set `category`/`accessLevel`, or `rotateToken` |
| `DELETE` | `/api/link/{name}` | Remove a link and its media |
| `POST` | `/api/link/{name}/pin` | Toggle pin |
| `POST` | `/api/upload` | Multipart `linkName` + `file` or `url` |
| `GET` | `/api/preview/{name}` | Protected thumbnail |
| `GET` | `/api/external-images` | Browse external files |
| `GET` | `/api/external-image-preview?path=...` | Preview an external image |
| `GET` | `/api/compression-config` | Current compression settings |
| `POST` | `/api/regenerate-previews` | Rebuild thumbnails |
| `GET` | `/health`, `/health/ready` | Public health and readiness checks |
| `GET` | `/{name}` | Media; subject to the link's access level |

Without `page`, `/api/wallpapers` returns an array as before. With `page`, it returns `{data, total, page, pageSize, totalPages}`. For example:

```sh
curl -u admin:"$ADMIN_PASS" 'http://localhost:8080/api/wallpapers?page=1&page_size=50'
curl -u admin:"$ADMIN_PASS" -X PATCH 'http://localhost:8080/api/link/bedroom' \
  -H 'Content-Type: application/json' -d '{"accessLevel":"token"}'
```

The browser's JSON export/import is **not** a full media backup: import creates missing link names and restores local UI preferences but does not import images, settings of existing links or access tokens. Exports can contain token secrets; protect them. Back up the entire persistent `data/` directory (including `wallpapers.json`, `media/` and `previews/`) for disaster recovery.

### Upgrading old installations

Existing `static/images/` media is moved to `data/media/` and `data/previews/` at startup. If old media was on a separate volume, make it available under `static/images/` for the first migration; then back up the new `data/` directory. Static URLs to old files are intentionally blocked to prevent bypassing link access rules. Corrupt or unsafe link metadata causes startup to fail instead of silently wiping it. Keep a backup before upgrading.

## Security and development

Run behind HTTPS, use strong credentials, restrict access to the backend when using an external auth proxy and monitor disk/memory. See [SECURITY.md](SECURITY.md) for actual security controls, their limits and how to report an issue; see [IMPROVEMENTS.md](IMPROVEMENTS.md) for this audit's changes and follow-ups.

```sh
go test ./...
go test -race ./...
go vet ./...
go mod verify
node --test tests/*.test.cjs
docker build -t lanpaper .
```

Docker builds and browser smoke checks require Docker/a browser respectively; neither is part of `go test`. The Docker image uses a non-root user. Go 1.25.3+ and CGO are required to build the WebP encoder. License: MIT (see [LICENSE](LICENSE)).
