# Lanpaper HTTP API

All endpoints are served by the single Lanpaper process (default port `8080`).
Examples use `curl` and assume `ADMIN_PASS` is set in the shell.

## Conventions

- **Authentication.** Admin endpoints (`/admin`, `/api/*`) use HTTP Basic Auth
  with `ADMIN_USER` / `ADMIN_PASS`.
  - Missing credentials: `401` with `WWW-Authenticate`.
  - Credentials not configured on the server: `503` (fail closed).
  - Lockout: after 10 wrong username/password pairs from one client within
    15 minutes, requests with credentials get `429` plus `Retry-After` until
    the window ends. Even correct credentials are rejected during the
    lockout. IPv6 clients are grouped by `/64`.
  - `DISABLE_AUTH=true` turns built-in auth off. Use it only behind an
    authenticating reverse proxy.
- **CSRF.** Browsers must send same-origin `POST`/`PATCH`/`DELETE` requests,
  as indicated by `Sec-Fetch-Site` / `Origin`. Otherwise the server answers
  `403`. Non-browser clients that send neither header are accepted, but still
  need Basic Auth.
- **Bodies.** JSON request bodies are limited to 64 KiB, and trailing data is
  rejected. Unknown fields are ignored.
- **Errors** are short plain-text messages, for example `Link not found`.
- **Rate limits** are counted per client, with IPv6 grouped by `/64`, in fixed
  one-minute windows:

  | Traffic | Requests per minute | Default |
  | --- | --- | --- |
  | Public links | `RATE_PUBLIC_PER_MIN + RATE_BURST` | 120 + 10 |
  | Uploads and preview regeneration | `RATE_UPLOAD_PER_MIN + RATE_BURST` | 20 + 10 |

  A limit of `0` disables that limiter. When a limit is exceeded, the server
  answers `429` with `Retry-After`.
- **Link names** are 1–64 characters: ASCII letters, digits, `_` and `-`,
  starting with a letter or digit. The following names are reserved: `api`,
  `admin`, `static`, `external`, `data`, `health`, `sw.js`, `favicon.ico`,
  `robots.txt`, `sitemap.xml`, `manifest.json`, `manifest.webmanifest`
  (case-insensitive).

## Link object

The API returns links in this shape:

```json
{
  "id": "bedroom",
  "linkName": "bedroom",
  "category": "other",
  "hasImage": true,
  "imageUrl": "/bedroom",
  "preview": "/api/preview/bedroom",
  "mimeType": "jpg",
  "sizeBytes": 482113,
  "modTime": 1790620000,
  "createdAt": 1790610000,
  "pinned": false,
  "pinnedAt": 1790630000,
  "accessLevel": "token",
  "accessToken": "k3J…"
}
```

| Field | Notes |
| --- | --- |
| `mimeType` | Stored file extension: `jpg`, `png`, `gif`, `webp`, `bmp`, `tiff`, `mp4` or `webm`. |
| `preview` | Absent for videos and for links without media. |
| `pinnedAt` | Present only for pinned links. |
| `accessToken` | Present only when `accessLevel` is `token`. |
| `category` | One of `tech`, `life`, `work`, `other`; defaults to `other`. |
| Timestamps | Unix seconds. |

## Admin endpoints

### `GET /api/wallpapers`

Lists links. By default, pinned links come first (most recently pinned
first), then links with media (newest first), then empty links.

| Query parameter | Meaning |
| --- | --- |
| `category` | Filter by category (case-insensitive). |
| `has_image` | `true` or `false`. |
| `sort` | `created` or `updated`. Pinned links stay first. |
| `order` | `desc` (default) or `asc`. |
| `page`, `page_size` | Optional pagination. `page_size` defaults to 50, maximum 200. |

- Without `page`, the response is a JSON array of link objects.
- With `page`, the response is
  `{"data": [...], "total": n, "page": p, "pageSize": s, "totalPages": t}`.
  An invalid page number returns `400`.

```sh
curl -u admin:"$ADMIN_PASS" 'http://localhost:8080/api/wallpapers?page=1&page_size=50'
```

### `POST /api/link`

Creates an empty link.

```sh
curl -u admin:"$ADMIN_PASS" -H 'Content-Type: application/json' \
  -d '{"linkName":"bedroom","accessLevel":"public"}' http://localhost:8080/api/link
```

- Body fields: `linkName` (required), `category` (optional) and
  `accessLevel` (optional, default `public`).
- Choosing `token` generates a random 256-bit token.
- Success: `201` with the link object.

| Status | Message |
| --- | --- |
| `400` | `Invalid link name` / `Invalid category` / `Invalid access level` / `Invalid JSON` |
| `409` | `Link name already taken` |

### `PATCH /api/link/{name}`

Changes one link. The body contains any of these fields:

| Field | Effect |
| --- | --- |
| `newLinkName` | Renames the link and its files. When present, the other fields are ignored, so send them in a separate request. |
| `category` | Sets the category. An empty string resets it to `other`. |
| `accessLevel` | `public`, `local`, `token` or `auth`. Switching to `token` generates a token, and an existing token is kept. Leaving `token` deletes the token. |
| `rotateToken` | `true` issues a new token, and the old one stops working immediately. On a link that is not token-protected this returns `400`. |

- Success: `200` with the updated link object.
- Errors: `404` if the link doesn't exist, `409` if the new name is taken.

```sh
curl -u admin:"$ADMIN_PASS" -X PATCH -H 'Content-Type: application/json' \
  -d '{"accessLevel":"token"}' http://localhost:8080/api/link/bedroom
```

### `DELETE /api/link/{name}`

Deletes the link together with its media and preview.

- Success: `204`.
- Error: `404` if the link doesn't exist.

### `POST /api/link/{name}/pin`

Toggles the pin. Returns `200` with the updated link object.

### `POST /api/upload`

Sets or replaces the media of an existing link. The request is
`multipart/form-data` with these fields:

- `linkName`: required. The link must already exist; otherwise the server
  answers `400 Link does not exist`.
- One media source:
  - `file`: an uploaded file.
  - `url`: a public `http(s)://` URL.
  - `url` set to a path relative to `EXTERNAL_IMAGE_DIR`, as returned by
    `/api/external-images`.

```sh
curl -u admin:"$ADMIN_PASS" -F linkName=bedroom -F file=@photo.jpg http://localhost:8080/api/upload
curl -u admin:"$ADMIN_PASS" -F linkName=bedroom -F url=https://example.com/photo.jpg http://localhost:8080/api/upload
```

Media handling:

- The type is detected from the file content, not from the file name or the
  `Content-Type` header.
- Images are fully decoded before they are stored.
- With `COMPRESSION_QUALITY=100` and `COMPRESSION_SCALE=100`, the original
  bytes are kept. Otherwise images are scaled and re-encoded.
- A WebP thumbnail is generated for each image.

Success: `200` with the updated link object.

| Status | Cause |
| --- | --- |
| `400` | Invalid input, unsupported or corrupt media, or a remote URL that is not allowed or failed to download. |
| `403` / `404` | Gallery path outside the gallery directory / file not found. |
| `413` | Larger than `MAX_UPLOAD_MB`. |
| `429` | Rate limit, concurrent-upload limit, or image memory budget exhausted (`Retry-After: 5`). |
| `500` | Storage error. |

Long uploads are allowed: after authentication, the read deadline is at least
120 s plus the time needed to transfer the maximum size at 256 KiB/s.

### `GET /api/preview/{name}`

Returns the WebP thumbnail. For videos, or when the thumbnail is missing, it
returns the original media. Always admin-only, whatever the link's access
level.

### `GET /api/external-images`

Returns a JSON array of media paths below `EXTERNAL_IMAGE_DIR`, relative and
slash-separated, for example `["holiday/beach.jpg"]`. The listing:

- Skips hidden files and directories.
- Skips files larger than `MAX_UPLOAD_MB`.
- Does not descend deeper than `MAX_WALK_DEPTH`.
- Ignores symlinks that resolve outside the directory.
- Stops at 5,000 files.

### `GET /api/external-image-preview?path=...`

Returns one gallery file after validating its type.

| Status | Cause |
| --- | --- |
| `400` | Invalid path or media. |
| `403` | Outside the gallery or unavailable. |
| `404` | Missing. |

### `GET /api/compression-config`

Returns `{"quality": 85, "scale": 100}`. The admin panel reads these values
for browser-side JPEG re-encoding.

### `POST /api/regenerate-previews`

Rebuilds all image thumbnails and removes orphaned ones. The response is
`{"total": n, "ok": n, "skipped": n, "errors": n, "failed": ["name", ...]}`.

- Only one run at a time; a second request while one is running returns `429`.
- Counts towards the upload rate limit.

## Public endpoints

### `GET /{name}` (and `HEAD`)

Serves the media of a link:

- `Content-Type`, `Content-Disposition: inline`, `Cache-Control: no-store`.
- HTTP range requests are supported.
- Cross-origin embedding is allowed (`Cross-Origin-Resource-Policy: cross-origin`).

| Access level | Requirement | Denied |
| --- | --- | --- |
| `public` | none | — |
| `local` | Client IP is loopback, RFC 1918, link-local, CGNAT or IPv6 ULA | `403` |
| `token` | `?token=…` or `X-Access-Token: …`; admin Basic Auth also works | `403` |
| `auth` | Admin Basic Auth | `401` (`403` with `DISABLE_AUTH=true`) |

- A link without media returns `404`.
- Wrong admin credentials count towards the login lockout.

### `GET /health`

Liveness check, always public. Returns `{"service":"lanpaper","status":"ok","version":"0.10.0"}`.

### `GET /health/ready`

Readiness check. It verifies that `data/` and `data/media/` are accessible
and that at least 1 GB of disk space is free.

- Ready: `200` with `{"status":"ready","checks":{...}}`.
- Not ready: `503`, with a `message` on each failing check.

### Other routes

| Route | Behaviour |
| --- | --- |
| `GET /admin` | Admin panel (Basic Auth). |
| `/admin.html` | Permanent redirect to `/admin`. |
| `/` | Redirect to `/admin`. |
| `GET /sw.js` | Service worker (root scope). |
| `GET /static/...` | Allowlisted application assets only. Legacy `static/images/` is never served. |
