# Lanpaper

> **One permanent link. Any content. Change it anytime.**

Create a link such as `https://your-server/tv` and assign an image or video to
it in the admin panel. You can change the content later without changing the
link, which is handy for digital frames, smart TVs, kiosks and other displays.

## Features

- **Stable named links.** The content can be JPEG, PNG, GIF, WebP, BMP, TIFF,
  MP4 or WebM, and can be swapped at any time.
- **Version history and rollback.** Replacing a file archives the previous one
  (three versions and a 512 MB global budget by default). Old versions stay
  reachable as `/{name}?v=2` and can be restored in one click — the rollback
  itself is archived, so it can be undone.
- **Playlists and rotation.** Several files can sit behind one URL: append them
  with `mode=append`, address one with `/{name}?i=2`, and let the link switch
  between them every N seconds, sequentially or at random. Rotation is derived
  from the clock, so it needs no worker, no state and no extra I/O.
- **Publishing without the admin login.** `PUBLISH_KEYS` accepts API keys that
  may only upload media and create links — never rename, re-scope, roll back,
  list or delete. `autoCreate=1` creates the link and pushes its first file in
  a single request, which is what a webhook or a cron job needs.
- **Readable from other origins.** `CORS_ORIGINS` allows selected sites to fetch
  public media with JavaScript, and `ALLOW_EMBED=true` lifts the framing
  restrictions for public media only (the admin panel stays unframable).
- **Three ways to add media:** upload a file, fetch a public HTTP(S) URL, or
  pick a file from a server-side gallery directory.
- **Image processing:** thumbnails, optional compression and scaling, and a
  lossless mode that keeps the original bytes. Memory stays bounded (about
  50 MB peak for a 24-megapixel photo) and is returned to the system after
  each upload.
- **Per-link access levels:** `public`, `local` (LAN only), `token` (secret
  URL) and `auth` (admin only). Media is stored outside the static web root.
- **Admin panel** (installable PWA) with gallery-style previews, search,
  filter chips with counts, sorting, pinning, drag-and-drop, five muted accent
  palettes, light and dark themes, export/import of the link list and six
  languages (EN, RU, DE, FR, IT, ES). The library is drawn in chunks of 12 with
  a "Show more" button, and a failed request gets its own state with a retry
  button instead of an empty list. App icons, portrait and panoramic images are
  shown whole instead of cropped; video previews play only while visible. A
  per-link panel manages media (a file from the device — pre-compressed in the
  browser — a remote URL or a file already on the server), versions with
  rollback, playlist items with rotation, and access levels with counters.
- **Light on resources:** about 10 MB of RAM when idle, gzip for text
  responses, media revalidated with `304` instead of downloaded again, and
  about 60 KB of self-hosted WOFF2 fonts. Archived versions are moved, not
  copied, and per-link access counters live in memory only.
- **Security:** Basic Auth with a brute-force lockout, CSRF protection,
  strict security headers, SSRF-safe downloads and rate limits. Publish keys
  are stored as SHA-256 digests, share the login lockout budget and are never
  written to `config.json`. A panicking handler is answered with a clean `500`
  and logged with its stack, so one bad request cannot take the process down.
- **Deployment:** a single static binary in a small non-root Docker image,
  with optional built-in TLS (`TLS_CERT_FILE` + `TLS_KEY_FILE`),
  HTTP/HTTPS/SOCKS5 proxy support for outbound downloads, health and readiness
  probes, graceful shutdown, and a `robots.txt` that keeps crawlers off mutable
  media URLs. See [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

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

You need Go 1.26 or newer (preferably the latest patch release). No C compiler
is required: WebP encoding is pure Go.

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

### Versions, playlists and rotation

One URL can serve more than one file:

| URL | What it serves |
| --- | --- |
| `/{name}` | The live file, or the playlist position the rotation clock currently points at |
| `/{name}?v=3` | Archived version 3 (`404` once it falls out of the history) |
| `/{name}?i=0` | The live file, whatever the rotation is doing |
| `/{name}?i=2` | Playlist item with id 2 (`404` after it is removed) |
| `/{name}.jpg`, `/{name}/latest` | Cosmetic aliases for `/{name}` |

- **History is on by default** with `HISTORY_LIMIT=3` versions per link and a
  `HISTORY_MAX_MB=512` budget shared by all links; the oldest archives are
  dropped first. Set `HISTORY_LIMIT=0` to keep the old replace-only behaviour.
- **A rollback is reversible:** the file that was live is archived by the same
  operation, so restoring version 2 creates version 5 rather than losing
  anything.
- **Playlist items have no thumbnails** on purpose: generating a WebP per item
  would multiply upload cost for metadata nobody looks at. The admin panel
  shows one preview per link.
- **Rotation is stateless.** The position is derived from
  `floor(now / interval)`, so two replicas reading the same `data/` directory
  always agree, nothing runs in the background, and the image cannot flicker
  between two requests inside the same interval. The interval is 5–86400
  seconds (default 60).

### Publishing from scripts

```bash
# Create the link and push its first file in one request.
curl -X POST https://your-server/api/upload \
  -H "X-Api-Key: $PUBLISH_KEY" \
  -F linkName=frame -F autoCreate=1 -F accessLevel=public \
  -F file=@photo.jpg

# Replace the content of an existing link; the previous file is archived.
curl -X POST https://your-server/api/upload \
  -H "X-Api-Key: $PUBLISH_KEY" -F linkName=frame -F file=@photo.jpg

# Add a second file behind the same URL instead of replacing it.
curl -X POST https://your-server/api/upload \
  -H "X-Api-Key: $PUBLISH_KEY" -F linkName=frame -F mode=append -F file=@second.jpg
```

`Authorization: Bearer <key>` works as well. A key authorizes **only**
`POST /api/upload` and `POST /api/link`; everything else still needs the admin
login, so a leaked publish key cannot read the library, change access levels or
delete anything. Keys come from the `PUBLISH_KEYS` environment variable only
(comma-separated, at least 16 characters, at most 32 keys) and are kept as
SHA-256 digests in memory — they never reach `config.json`. Wrong keys share
the brute-force budget with admin logins, so guessing is locked out per client.

A publish key can **add** media but never **replace** it. `POST /api/upload`
with a key creates a new link and uploads its first file, or appends a playlist
item with `mode=append`. A key that asks for the default `mode=replace` on a
link that already has media gets `403 Publish keys cannot replace existing
media`. Replacing a live file is an admin action, so a leaked key cannot deface
links that are already in use.

**Behind a reverse proxy**, set `TRUSTED_PROXY` to *only* the address the
proxy reaches Lanpaper from — one IP or CIDR, or a comma-separated list when
the proxy arrives through more than one hop (a Docker container, for example,
sees the bridge gateway and not the proxy's LAN address:
`TRUSTED_PROXY="192.168.20.1,172.24.0.1"`). Configure the proxy to
**append to or overwrite** `X-Forwarded-For`, and **overwrite**
`X-Forwarded-Proto` and `X-Forwarded-Host`, and to preserve the original `Host`.
Lanpaper reads only the rightmost `X-Forwarded-For` entry from the trusted
proxy and never reads `X-Real-IP`. Without this:

- every visitor appears to come from the proxy's (often private) address, so
  `local` links become reachable for everyone;
- every visitor shares one rate-limit and login-lockout bucket.

A proxy that terminates TLS but forwards no `X-Forwarded-Proto` is the one
exception: the browser's `https` origin then meets a plain-HTTP connection, and
only a request the browser itself marks as same-origin (`Sec-Fetch-Site:
same-origin`) whose host and port fit the other scheme is accepted — the
classic case of a panel behind a small TLS terminator that adds no headers.
Forwarding the header keeps the scheme check exact for every client, including
older browsers that send no `Sec-Fetch-Site`.

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
| `HISTORY_LIMIT` | `3` | Archived versions kept per link, 0–50. `0` disables version history. |
| `HISTORY_MAX_MB` | `512` | Global budget for archives in MiB; the oldest are dropped first. `0` disables the budget. |
| `PLAYLIST_MAX` | `8` | Extra files one link can serve from the same URL, 1–64. |
| `MAX_CONCURRENT_UPLOADS` | `2` | 1–8 |
| `MAX_WALK_DEPTH` | `3` | Gallery directory depth, 1–10 |
| `EXTERNAL_IMAGE_DIR` | `external/images` | Server-side gallery. Mount it read-only if possible. |
| `RATE_PUBLIC_PER_MIN` | `120` | Public link requests per client per minute (`0` disables) |
| `RATE_UPLOAD_PER_MIN` | `20` | Uploads and preview regenerations per client per minute (`0` disables) |
| `RATE_BURST` | `10` | Extra requests allowed per window |
| `CORS_ORIGINS` | unset | Comma-separated browser origins allowed to read public media; `*` allows any origin |
| `ALLOW_EMBED` | `false` | Lets other sites frame public media: drops `X-Frame-Options` and the CSP `sandbox` for `/{name}` only |
| `PUBLISH_KEYS` | unset | Comma-separated API keys (16+ characters, max 32) that may create links and add media (`mode=append` or a new link), never replace an existing link's file. Environment only — never written to `config.json` |
| `COMPRESSION_QUALITY` | `85` | JPEG/WebP quality, 1–100 |
| `COMPRESSION_SCALE` | `100` | Stored image size as a percentage of the original, 1–100 |
| `TRUSTED_PROXY` | unset | Address the reverse proxy connects from: an IP or CIDR, or a comma-separated list of them when there is more than one hop. Forwarded headers are trusted only from these. |
| `TLS_CERT_FILE`, `TLS_KEY_FILE` | unset | Serve HTTPS from the process itself (TLS 1.2+). Both or neither: a half-configured certificate is a startup error, not a silent downgrade to plaintext. |
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

Admin endpoints use Basic Auth, or a `PUBLISH_KEYS` API key for the two
publishing routes (`POST /api/upload`, `POST /api/link`). The full reference,
with request and response formats, is in [docs/API.md](docs/API.md).

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/wallpapers` | List links (optional filters, sorting, pagination) |
| `POST` | `/api/link` | Create a link: `{"linkName":"bedroom"}` |
| `PATCH` | `/api/link/{name}` | Rename, set `category` / `accessLevel`, `rotateToken`, `rotate` or `removeItem` |
| `DELETE` | `/api/link/{name}` | Delete a link, its media, its archive and its playlist |
| `POST` | `/api/link/{name}/pin` | Toggle pin |
| `GET` | `/api/link/{name}/history` | List archived versions plus the live one |
| `POST` | `/api/link/{name}/rollback` | Restore a version: `{"version":2}` |
| `DELETE` | `/api/link/{name}/history/{version}` | Drop one archived version and free its space |
| `POST` | `/api/upload` | Multipart `linkName` plus `file` or `url`, optional `mode=append` and `autoCreate=1` |
| `GET` | `/api/preview/{name}` | Admin thumbnail |
| `GET` | `/api/external-images` | List gallery files |
| `GET` | `/api/external-image-preview?path=…` | Fetch one gallery file |
| `GET` | `/api/compression-config` | Current compression settings |
| `POST` | `/api/regenerate-previews` | Rebuild thumbnails |
| `GET` | `/health`, `/health/ready` | Public liveness and readiness checks |
| `GET` | `/{name}` | The link's media, subject to its access level. `?v=N` picks an archived version, `?i=N` a playlist item, and `.jpg`/`/latest` suffixes are aliases |

```sh
curl -u admin:"$ADMIN_PASS" -F linkName=bedroom -F file=@photo.jpg http://localhost:8080/api/upload
curl -u admin:"$ADMIN_PASS" -X PATCH -H 'Content-Type: application/json' \
  -d '{"accessLevel":"token"}' http://localhost:8080/api/link/bedroom
curl -u admin:"$ADMIN_PASS" http://localhost:8080/api/link/bedroom/history
curl -u admin:"$ADMIN_PASS" -X POST -H 'Content-Type: application/json' \
  -d '{"version":2}' http://localhost:8080/api/link/bedroom/rollback
curl -H "X-Api-Key: $PUBLISH_KEY" -F linkName=frame -F autoCreate=1 -F file=@photo.jpg \
  http://localhost:8080/api/upload
```

## Production deployment

[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) is the operational guide: topology
(reverse proxy vs. built-in TLS), a configuration checklist, Docker and a
hardened systemd unit, which log lines deserve an alert, capacity planning,
backups, and a pre-launch checklist.

The short version: terminate TLS at a reverse proxy and set `TRUSTED_PROXY` to
the address it reaches Lanpaper from (a list, if there is more than one hop),
keep `data/` on a volume that is actually backed up, use a long
random `ADMIN_PASS`, set `HISTORY_MAX_MB` to what the volume can spare, and
read every `Warning:` the process prints at startup.

Two properties worth knowing before sizing a host:

- **Authentication is stateless.** There are no sessions or cookies: the admin
  password is sent with every request as HTTP Basic Auth, so TLS in front (or
  `TLS_CERT_FILE`/`TLS_KEY_FILE`) is what keeps it private. Failed logins are
  counted per client and locked out for a while.
- **Image work is bounded.** One image may hold 36 M pixels and at most 48 M
  decoded pixels may be in flight, so a burst of uploads queues instead of
  growing the process without limit. A 36 M pixel upload peaks at about 174 MB
  of memory with the pure Go WebP encoder.

## Backups and upgrades

Back up the whole persistent `data/` directory: `wallpapers.json`, `media/`,
`previews/` and, if you use them, `history/` (archived versions) and `items/`
(playlist files). Skipping the last two loses only the extra copies: the live
media of every link stays in `media/`.

The browser's JSON export is **not** a media backup:

- It contains link names and UI preferences, without access tokens.
- Importing it only creates the missing links. It never replaces media or
  existing links.

**Upgrading from 0.11.x:**

- Existing data keeps working: `history/` and `items/` are created on demand
  and every new field in `wallpapers.json` is optional, so a link that uses
  neither feature is stored exactly as before.
- Version history is on by default (`HISTORY_LIMIT=3`, 512 MB budget). Set
  `HISTORY_LIMIT=0` if you want the previous replace-only behaviour and no
  extra disk use.
- Downgrading after using these features leaves `history/` and `items/` on
  disk; older versions ignore them, so delete the directories to reclaim space.

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
node --test tests/*.test.cjs # API surface, UI contracts and i18n parity
docker build -t lanpaper .
```

CI runs the same checks on Go `oldstable` and `stable`. It then builds the
Docker image, smoke-tests it, and publishes `linux/amd64` and `linux/arm64`
images from `main`. Pushing a version tag runs the same gate and creates the
GitHub release (see `.github/workflows/release.yml`).

Contributing — invariants, compatibility rules and review bar:
[CONTRIBUTING.md](CONTRIBUTING.md).

## Licensing and commercial use

Lanpaper is **MIT licensed** ([LICENSE](LICENSE)): you may use it, modify it,
redistribute it and sell it, including as a paid hosted service, as long as the
copyright notice and the licence text travel with your copies. It comes without
warranty, and no trademark rights are granted.

Every dependency is permissive too — BSD-3-Clause, MIT, and SIL OFL 1.1 for the
bundled font — and nothing copyleft is linked into the binary or served by
it, so a commercial offering carries no source-disclosure obligation. The
component list, with the licence texts a redistribution has to reproduce, is in
[THIRD-PARTY-NOTICES.md](THIRD-PARTY-NOTICES.md). Every first-party source file
carries an `SPDX-License-Identifier: MIT` header, so licence scanning tools can
verify this mechanically.

Planning a proprietary or open-core licence later? Set up a CLA (or copyright
assignment) **before** accepting outside contributions: MIT-inbound code cannot
be relicensed unilaterally.

Security reports: see [SECURITY.md](SECURITY.md). Deployment and operations:
see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).
