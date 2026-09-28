# Lanpaper

> **One permanent link. Any content. Change it anytime.**

Create a link such as `https://your-server/tv` and assign an image or video to
it in the admin panel. You can change the content later without changing the
link, which is handy for digital frames, smart TVs, kiosks and other displays.

## Features

- **Stable named links.** The content can be JPEG, PNG, GIF, WebP, BMP, TIFF,
  MP4 or WebM, and can be swapped at any time.
- **Three ways to add media:** upload a file, fetch a public HTTP(S) URL, or
  pick a file from a server-side gallery directory.
- **Image processing:** thumbnails, optional compression and scaling, and a
  lossless mode that keeps the original bytes.
- **Per-link access levels:** `public`, `local` (LAN only), `token` (secret
  URL) and `auth` (admin only). Media is stored outside the static web root.
- **Admin panel** (installable PWA) with search, sorting, pinning,
  drag-and-drop, dark mode, export/import of the link list and six languages
  (EN, RU, DE, FR, IT, ES).
- **Security:** Basic Auth with a brute-force lockout, CSRF protection,
  strict security headers, SSRF-safe downloads and rate limits.
- **Deployment:** a single static binary in a small non-root Docker image,
  with optional HTTP/HTTPS/SOCKS5 proxy support for outbound downloads.

## Quick start

### Docker Compose

```sh
cp docker-compose-example.yml docker-compose.yml
printf 'ADMIN_PASS=%s\n' "$(openssl rand -hex 24)" > .env
chmod 600 .env
mkdir -p data && sudo chown 100:101 data   # the container runs as uid 100 / gid 101
docker compose up -d
```

Open <http://localhost:8080/admin> and log in as `admin` with the generated
password from `.env`.

- The example publishes port 8080 on all interfaces. Put HTTPS in front
  (see [SECURITY.md](SECURITY.md#reverse-proxy-examples)) before exposing it
  to the internet.
- To expose it only to a local reverse proxy, bind the port as
  `127.0.0.1:8080:8080`.
- Keep `.env` private; Git ignores it.

### Docker run

```sh
: "${ADMIN_PASS:?Set a unique, long ADMIN_PASS in your shell first}"
mkdir -p data && sudo chown 100:101 data
docker run -d --name lanpaper -p 8080:8080 \
  --cap-drop ALL --security-opt no-new-privileges:true --stop-timeout 35 \
  -e ADMIN_USER=admin -e ADMIN_PASS="$ADMIN_PASS" \
  -v "$(pwd)/data:/app/data" ptabi/lanpaper:latest
```

The `chown` is needed for bind mounts on Linux. It is not needed for Docker
named volumes or Docker Desktop.

### Build from source

You need Go 1.26 or newer (preferably the latest patch release) and a C
compiler (CGO is used for WebP encoding).

```sh
go build -trimpath -o lanpaper .
ADMIN_USER=admin ADMIN_PASS='set-a-unique-long-password' ./lanpaper
```

If credentials are missing, **admin routes return HTTP 503**; they are never
made public. To use an external authentication proxy instead:

1. Explicitly set `DISABLE_AUTH=true`.
2. Block direct access to the backend.
3. Enforce authentication on `/admin` **and** `/api/*` at the proxy.

Public links and `/health` stay reachable without admin credentials.

## Usage and access control

1. Create a name, e.g. `bedroom`, in `/admin`.
2. Upload a file, enter a public URL, or pick a file from `EXTERNAL_IMAGE_DIR`.
3. Open `https://your-server/bedroom` on the display. Later uploads keep the
   same URL.

Link name rules:

- Start with an ASCII letter or digit.
- Use only letters, digits, `_` and `-`.
- At most 64 characters.

A new link has no media until the first upload.

| Access level | Who can open `GET /{name}` |
| --- | --- |
| `public` (default) | Anyone who knows the URL |
| `local` | Clients on loopback, private/LAN, link-local, CGNAT or IPv6 ULA networks |
| `token` | Requests with a valid `?token=` or `X-Access-Token` header; admin Basic Auth also works |
| `auth` | Admin Basic Auth only |

Tokens:

- A token is generated when you select the `token` level.
- You can rotate it in the UI or through the API; the old token stops working
  immediately.
- Token URLs are secrets, so keep them out of logs and analytics.

The admin preview endpoint always requires admin credentials. With
`DISABLE_AUTH=true`, `auth`-level links are not served at `/{name}`.

**Behind a reverse proxy**, set `TRUSTED_PROXY` to *only* the proxy's IP or
CIDR. Configure the proxy to **overwrite** `X-Real-IP`, `X-Forwarded-For`,
`X-Forwarded-Proto` and `X-Forwarded-Host`, and to preserve the original
`Host`. Without this:

- every visitor appears to come from the proxy's (often private) address, so
  `local` links become reachable for everyone;
- every visitor shares one rate-limit and login-lockout bucket.

See the nginx and Caddy examples in [SECURITY.md](SECURITY.md#reverse-proxy-examples).

## Configuration

Settings are read in this order: built-in defaults, then an optional
`config.json` in the working directory (field names:
[config.example.json](config.example.json)), then environment variables,
which have the highest priority.

- Changes require a restart.
- Invalid numbers or booleans are logged and ignored.
- Out-of-range values fall back to the default.

| Environment variable | Default | Notes |
| --- | --- | --- |
| `PORT` | `8080` | Listening port |
| `ADMIN_USER`, `ADMIN_PASS` | unset | Both required. Missing credentials deny admin access (503). |
| `DISABLE_AUTH` | `false` | Explicit opt-out for an external auth proxy. Dangerous if misused. |
| `MAX_UPLOAD_MB` | `50` | Per file, 1–512 MiB |
| `MAX_IMAGES` | `0` | `0` = unlimited. Otherwise the oldest non-pinned media is pruned. |
| `MAX_CONCURRENT_UPLOADS` | `2` | 1–8 |
| `MAX_WALK_DEPTH` | `3` | Gallery directory depth, 1–10 |
| `EXTERNAL_IMAGE_DIR` | `external/images` | Server-side gallery. Mount it read-only if possible. |
| `RATE_PUBLIC_PER_MIN` | `120` | Public link requests per client per minute (`0` disables) |
| `RATE_UPLOAD_PER_MIN` | `20` | Uploads and preview regenerations per client per minute (`0` disables) |
| `RATE_BURST` | `10` | Extra requests allowed per window |
| `COMPRESSION_QUALITY` | `85` | JPEG/WebP quality, 1–100 |
| `COMPRESSION_SCALE` | `100` | Stored image size as a percentage of the original, 1–100 |
| `TRUSTED_PROXY` | unset | IP or CIDR of the reverse proxy. Forwarded headers are trusted only from it. |
| `PROXY_TYPE` | `http` | Outbound proxy type: `http`, `https` or `socks5` |
| `PROXY_HOST`, `PROXY_PORT` | unset | Optional outbound proxy for URL downloads |
| `PROXY_USERNAME`, `PROXY_PASSWORD` | unset | Proxy credentials (aliases: `PROXY_USER`, `PROXY_PASS`) |
| `INSECURE_SKIP_VERIFY` | `false` | Skips outbound TLS verification. Development only. |

A `.env` file in the working directory is loaded as well. Variables that are
already set in the environment take precedence over it.

### Image formats and compression

- **Lossless mode.** With `COMPRESSION_QUALITY=100` **and**
  `COMPRESSION_SCALE=100`, images are fully decoded to validate them, and then
  their original bytes are stored unchanged. This keeps the format (including
  BMP/TIFF) and GIF animation.
- **Other settings.** The server decodes, scales by `COMPRESSION_SCALE` and
  re-encodes. BMP/TIFF become JPEG, and a re-encoded GIF keeps only its first
  frame. MP4/WebM are stored as they are.
- **Browser pre-processing.** To reduce upload size, the admin panel
  re-encodes **JPEGs** at the configured quality before uploading them:
  - It never resizes; scaling is always done by the server, the same way for
    file, URL and gallery uploads.
  - It sends the original instead when that is smaller.
  - It is skipped in lossless mode, when the settings can't be loaded, and
    for images above about 16.7 MP.
- **Unsupported and limits.** Animated WebP is not supported; static WebP is.
  Images are limited to 16,384 px per side, 36 M pixels per image and 48 M
  decoded pixels in flight. Large or malformed media may be rejected even
  below `MAX_UPLOAD_MB`.

### Remote URLs

- Only HTTP(S) URLs whose every DNS answer is a public address are allowed.
  Private, loopback, link-local, CGNAT, NAT64, 6to4 and Teredo ranges are
  blocked.
- The vetted IP is pinned for the connection, even through a configured
  proxy, and each redirect is checked again (at most 5).
- Downloads are streamed to bounded temporary files.
- A configured **HTTP proxy must route by the IP in the absolute request
  URI**, not by the `Host` header.
- Gallery paths never leave `EXTERNAL_IMAGE_DIR`.

## API

Admin endpoints use Basic Auth. The full reference, with request and response
formats, is in [docs/API.md](docs/API.md).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallpapers` | List links (optional filters, sorting, pagination) |
| `POST` | `/api/link` | Create a link: `{"linkName":"bedroom"}` |
| `PATCH` | `/api/link/{name}` | Rename, set `category` / `accessLevel`, or `rotateToken` |
| `DELETE` | `/api/link/{name}` | Delete a link and its media |
| `POST` | `/api/link/{name}/pin` | Toggle pin |
| `POST` | `/api/upload` | Multipart `linkName` plus `file` or `url` |
| `GET` | `/api/preview/{name}` | Admin thumbnail |
| `GET` | `/api/external-images` | List gallery files |
| `GET` | `/api/external-image-preview?path=…` | Fetch one gallery file |
| `GET` | `/api/compression-config` | Current compression settings |
| `POST` | `/api/regenerate-previews` | Rebuild thumbnails |
| `GET` | `/health`, `/health/ready` | Public liveness and readiness checks |
| `GET` | `/{name}` | The link's media, subject to its access level |

```sh
curl -u admin:"$ADMIN_PASS" -F linkName=bedroom -F file=@photo.jpg http://localhost:8080/api/upload
curl -u admin:"$ADMIN_PASS" -X PATCH -H 'Content-Type: application/json' \
  -d '{"accessLevel":"token"}' http://localhost:8080/api/link/bedroom
```

## Backups and upgrades

Back up the whole persistent `data/` directory: `wallpapers.json`, `media/` and
`previews/`.

The browser's JSON export is **not** a media backup:

- It contains link names and UI preferences, without access tokens.
- Importing it only creates the missing links. It never replaces media or
  existing links.

**Upgrading from 0.9.x:**

- Existing data keeps working.
- The Docker image still runs as uid 100 / gid 101, now pinned explicitly.
- Building from source needs Go 1.26+.

**Very old installations** stored media in `static/images/`. At startup it is
moved to `data/media/` and `data/previews/`.

- If that media was on a separate volume, mount it at `static/images/` for the
  first start, then back up the new `data/` directory.
- Old static URLs stay blocked, so they cannot bypass link access rules.
- Corrupt metadata stops startup instead of being silently wiped.

See [CHANGELOG.md](CHANGELOG.md) for release notes and [ROADMAP.md](ROADMAP.md)
for planned work.

## Development

```sh
gofmt -l .                  # must print nothing
go vet ./...
go test -race ./...
go mod verify
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
node --test tests/*.test.cjs
docker build -t lanpaper .
```

CI runs the same checks on Go `oldstable` and `stable`. It then builds the
Docker image, smoke-tests it, and publishes `linux/amd64` and `linux/arm64`
images from `main`.

Security reports: see [SECURITY.md](SECURITY.md). License: MIT, see
[LICENSE](LICENSE).
