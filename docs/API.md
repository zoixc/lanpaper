# Lanpaper HTTP API

All endpoints are served by the single Lanpaper process (default port `8080`).
Examples use `curl` and assume `ADMIN_PASS` is set in the shell.

## Conventions

- **Authentication.** Admin API endpoints (`/api/*`) accept the session cookie
  from `POST /api/session`; non-browser clients may instead send HTTP Basic
  Auth with `ADMIN_USER` / `ADMIN_PASS`. Requests with browser Fetch Metadata
  never fall back to Basic. The `/admin` browser page accepts the cookie only.
  `POST /api/session` takes `{"username":"…","password":"…"}` and returns
  `204` with the cookie. `DELETE /api/session` signs out the current browser;
  authenticated `GET /api/sessions` lists non-secret lifecycle metadata and
  `DELETE /api/sessions` signs out every browser session. Sessions are kept in
  `data/sessions.json`, so they survive a restart. Wrong passwords return `401` and share the Basic Auth
  lockout (`429`).
  - **Publish keys.** `PUBLISH_KEYS` (environment only, comma-separated, 16+
    characters, at most 32) authorizes *publishing* without the admin login:
    `POST /api/upload` and `POST /api/link`. Send the key as
    `X-Api-Key: <key>` or `Authorization: Bearer <key>`. Every other admin
    route — rename, access level, pin, history, rollback, delete, listing —
    still requires Basic Auth, so a key is not an admin session.
  - A key may add media but never replace it: `mode=replace` (the default)
    on a link that already has media returns `403`. Use `mode=append`, or an
    admin login, to change a live file.
  - Keys are kept as SHA-256 digests in memory, are never written to
    `config.json`, and appear in logs only as an 8-hex-character fingerprint.
  - A wrong key answers `401` and shares the brute-force budget with admin
    logins: after 10 failures from one client within 15 minutes the client gets
    `429` plus `Retry-After`, and even a correct key is refused until the
    window ends. When no keys are configured, an `X-Api-Key` header is ignored
    completely and cannot lock anybody out.
  - Missing API credentials: `401` with `WWW-Authenticate`. Public media at
    `/{name}` never sends a Basic challenge; session cookies and preemptive
    Basic credentials from non-browser clients are accepted for `auth` links.
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
- **Compression.** Text responses (HTML, CSS, JavaScript, JSON, SVG) of at
  least 1 KiB are gzip-compressed for clients that send
  `Accept-Encoding: gzip`, with `Vary: Accept-Encoding`. Media, range requests
  and `HEAD` responses are never compressed.
- **Caching.** The admin page and API responses are `Cache-Control: no-store`.
  Media responses carry `ETag` and `Last-Modified`, and conditional requests
  (`If-None-Match`, `If-Modified-Since`) get `304` — but only after the access
  check has passed.
- **CORS and embedding.** Public media is same-origin by default: no
  `Access-Control-Allow-Origin`, `X-Frame-Options: DENY` and a sandboxing CSP.
  `CORS_ORIGINS` (comma-separated, `*` for any origin) adds
  `Access-Control-Allow-Origin` for the listed origins plus `Vary: Origin`,
  `Access-Control-Expose-Headers` (`ETag`, `Last-Modified`, `Content-Length`,
  `Content-Range`, `Content-Type`) and answers `OPTIONS` preflights with `204`
  (`Access-Control-Allow-Methods: GET, HEAD, OPTIONS`, `Max-Age: 600`, and the
  requested headers filtered to an allowlist). Without `CORS_ORIGINS`, or for an
  origin that is not listed, `OPTIONS` keeps returning `405`.
  `ALLOW_EMBED=true` additionally drops `X-Frame-Options` and the CSP `sandbox`
  for `/{name}` only — `/admin` and `/api/*` stay unframable.
- **Selectors.** `/{name}` serves one file, chosen in this order: `?v=N` (an
  archived version), `?i=N` (a playlist item, `0` for the live file), otherwise
  the rotation clock, otherwise the live file. An unusable selector is a `404`;
  it never silently falls back to different bytes. `?v=` and `?i=` together are
  a `404`.
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
  "accessToken": "k3J…",
  "currentVersion": 4,
  "history": [
    { "version": 3, "ext": "jpg", "sizeBytes": 471220, "modTime": 1790615000, "savedAt": 1790620000 },
    { "version": 2, "ext": "png", "sizeBytes": 903114, "modTime": 1790612000, "savedAt": 1790615000 }
  ],
  "items": [ { "id": 1, "ext": "webm", "sizeBytes": 2211003, "modTime": 1790618000, "addedAt": 1790618000 } ],
  "rotate": { "enabled": true, "interval": 60, "order": "sequential" },
  "stats": { "hits": 42, "bytes": 20248746, "lastHit": 1790630000 }
}
```

| Field | Notes |
| --- | --- |
| `mimeType` | Stored file extension: `jpg`, `png`, `gif`, `webp`, `bmp`, `tiff`, `mp4` or `webm`. |
| `preview` | Absent for videos and for links without media. |
| `pinnedAt` | Present only for pinned links. |
| `accessToken` | Present only when `accessLevel` is `token`. |
| `category` | One of `tech`, `life`, `work`, `other`; defaults to `other` and is never the media kind. The file's own type is in `mimeType`. |
| `currentVersion` | Version number of the file `/{name}` serves. Absent for a link without media; a record written before versioning existed reads as `1`. |
| `history` | Archived versions, newest first. Absent when nothing is archived. |
| `items` | Playlist entries behind the same URL. The live file is not listed; it is position `0`. |
| `rotate` | Present only when the link has playlist items or rotation settings, so links that use neither serialize exactly as before. |
| `stats` | Requests and bytes served since the process started. In memory only: it resets on restart, holds no visitor identity, and is absent until the link has been requested. `304` and `HEAD` count as a hit with `0` bytes; refused requests (`401`, `403`, `404`, `429`) are not counted. |
| Timestamps | Unix seconds. |

## Admin endpoints

### `GET /api/wallpapers`

Lists links. By default, pinned links come first (most recently pinned
first), then links with media (newest first), then empty links.

| Query parameter | Meaning |
| --- | --- |
| `category` | Filter by category (case-insensitive). |
| `has_image` | `true`/`1` or `false`/`0`. Any other value returns `400`. |
| `sort` | `created` or `updated`; any other value returns `400`. Pinned links stay first. |
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
| `rotate` | `{"enabled":bool,"interval":seconds,"order":"sequential"\|"random"}`. Fields are merged with the stored settings, so sending only `enabled` keeps the rest. `interval: 0` restores the default (60 s); any other value must be 5–86400. An unknown `order` returns `400`. Rotation only has an effect while the link has playlist items. |
| `removeItem` | Playlist item id to delete. `0` or a negative id returns `400`; an id that does not exist returns `404`. The item's file is removed only after the metadata commit. |

- Success: `200` with the updated link object.
- Errors: `404` if the link (or the playlist item) doesn't exist, `409` if the
  new name is taken.
- Renaming a link moves `data/history/{name}/` and `data/items/{name}/` with it
  and rolls the move back if the metadata save fails.

```sh
curl -u admin:"$ADMIN_PASS" -X PATCH -H 'Content-Type: application/json' \
  -d '{"accessLevel":"token"}' http://localhost:8080/api/link/bedroom
curl -u admin:"$ADMIN_PASS" -X PATCH -H 'Content-Type: application/json' \
  -d '{"rotate":{"enabled":true,"interval":300,"order":"random"}}' http://localhost:8080/api/link/bedroom
curl -u admin:"$ADMIN_PASS" -X PATCH -H 'Content-Type: application/json' \
  -d '{"removeItem":2}' http://localhost:8080/api/link/bedroom
```

### `DELETE /api/link/{name}`

Deletes the link together with its media, preview, archived versions
(`data/history/{name}/`), playlist items (`data/items/{name}/`) and its access
counters.

- Success: `204`.
- Error: `404` if the link doesn't exist.

### `POST /api/link/{name}/pin`

Toggles the pin. Returns `200` with the updated link object.

### `GET /api/link/{name}/history`

Lists what the URL serves and what can be restored.

```json
{
  "linkName": "bedroom",
  "currentVersion": 4,
  "live": { "version": 4, "ext": "jpg", "sizeBytes": 482113, "modTime": 1790620000, "savedAt": 1790620000 },
  "history": [ { "version": 3, "ext": "jpg", "sizeBytes": 471220, "modTime": 1790615000, "savedAt": 1790620000 } ],
  "limit": 3,
  "bytes": 471220
}
```

| Field | Notes |
| --- | --- |
| `live` | The file `/{name}` serves, in the same shape as an archived version. Zero-valued for a link without media. |
| `history` | Archived versions, newest first. `[]` when nothing is archived. |
| `limit` | `HISTORY_LIMIT`. `0` means version history is disabled server-wide. |
| `bytes` | Size of the listed archives for this link. The server-wide budget is `HISTORY_MAX_MB`. |

- Errors: `404` if the link doesn't exist, `405` for any method but `GET`.

```sh
curl -u admin:"$ADMIN_PASS" http://localhost:8080/api/link/bedroom/history
```

### `POST /api/link/{name}/rollback`

Restores an archived version as the file the URL serves.

- Body: `{"version":2}` — the version must be listed in `history`.
- The file that was live is archived by the same operation, so a rollback can
  itself be rolled back: restoring version 2 of a link at version 4 makes the
  live file version 5 and archives the displaced file as version 4.
- The thumbnail is regenerated from the restored file (a video clears it).
- Success: `200` with the updated link object.

| Status | Cause |
| --- | --- |
| `400` | `Invalid version` (missing or `0`) or `Invalid JSON`. |
| `404` | The link doesn't exist, the version isn't archived, or its file is gone. |

```sh
curl -u admin:"$ADMIN_PASS" -X POST -H 'Content-Type: application/json' \
  -d '{"version":2}' http://localhost:8080/api/link/bedroom/rollback
```

### `DELETE /api/link/{name}/history/{version}`

Drops one archived version and deletes its file immediately.

- Success: `200` with the updated link object.
- Errors: `400` for an invalid name or version, `404` when the version isn't
  archived. Removing the last version also removes `data/history/{name}/`.

```sh
curl -u admin:"$ADMIN_PASS" -X DELETE http://localhost:8080/api/link/bedroom/history/3
```

### `POST /api/upload`

Sets or replaces the media of an existing link, or adds a playlist item to it.
The request is `multipart/form-data` with these fields:

- `linkName`: required. The link must already exist; otherwise the server
  answers `400 Link does not exist` (see `autoCreate`).
- One media source:
  - `file`: an uploaded file.
  - `url`: a public `http(s)://` URL.
  - `url` set to a path relative to `EXTERNAL_IMAGE_DIR`, as returned by
    `/api/external-images`.
- `mode`: `replace` (default) or `append`.
  - `replace` swaps the live file and archives the previous one when
    `HISTORY_LIMIT` is greater than `0`.
  - `append` stores the file as a playlist item behind the same URL and leaves
    the live file — and every existing embed — untouched. It needs a link that
    already has media (`400` otherwise), does not generate a thumbnail, and
    answers `409 Playlist is full` at `PLAYLIST_MAX` items.
- `autoCreate`: `1`, `true`, `yes` or `on` creates the link when it does not
  exist yet, so one request can create a link and push its first file. Optional
  `category` and `accessLevel` fields are validated and applied to the new link
  (`400 Invalid access level or category` otherwise). Without the flag an
  unknown name is still rejected, so a typo cannot silently create a link.
- Authentication: Basic Auth, or a publish key (`X-Api-Key` /
  `Authorization: Bearer`) when `PUBLISH_KEYS` is set.

```sh
curl -u admin:"$ADMIN_PASS" -F linkName=bedroom -F file=@photo.jpg http://localhost:8080/api/upload
curl -u admin:"$ADMIN_PASS" -F linkName=bedroom -F url=https://example.com/photo.jpg http://localhost:8080/api/upload
curl -H "X-Api-Key: $PUBLISH_KEY" -F linkName=frame -F autoCreate=1 -F file=@photo.jpg http://localhost:8080/api/upload
curl -H "X-Api-Key: $PUBLISH_KEY" -F linkName=frame -F mode=append -F file=@second.jpg http://localhost:8080/api/upload
```

Media handling:

- The type is detected from the file content, not from the file name or the
  `Content-Type` header.
- Accepted containers are the stored extensions above plus the MP4 brands real
  cameras write (`isom`, `iso2`, `avc1`, `M4V`…). ISO-BMFF *image* containers
  are not decoded and are refused with `400 Invalid or unsupported media file`:
  HEIF/AVIF (`avif`, `avis`, `heic`, `heix`, `heim`, `heis`, `hevc`, `hevx`,
  `hevm`, `hevs`, `mif1`, `msf1`, `msix`, `mshf`).
- Images are fully decoded before they are stored.
- With `COMPRESSION_QUALITY=100` and `COMPRESSION_SCALE=100`, the original
  bytes are kept. Otherwise images are scaled and re-encoded.
- A WebP thumbnail is generated for each image (`mode=append` skips it).
- The replaced file is moved into `data/history/{name}/{version}.{ext}` — a
  rename, not a copy — only after the metadata commit succeeded. A failed
  archive is logged and never fails the upload.

Success: `200` with the updated link object.

| Status | Cause |
| --- | --- |
| `400` | Invalid input, unsupported or corrupt media, an unknown `mode`, an `append` to a link without media, invalid `autoCreate` defaults, or a remote URL that is not allowed or failed to download. |
| `401` | Missing or wrong credentials, or a wrong publish key. |
| `403` | A publish key tried to replace the media of a link that already has media, a cross-origin request was refused, or a gallery path lies outside the gallery directory. |
| `404` | File not found. |
| `409` | The playlist already holds `PLAYLIST_MAX` items. |
| `413` | Larger than `MAX_UPLOAD_MB`. |
| `429` | Rate limit, concurrent-upload limit (`MAX_CONCURRENT_UPLOADS`, default 2 — the slots are busy, not the rate: retry when one finishes), or image memory budget exhausted. All of them carry `Retry-After`. |
| `500` | Storage error. |

Long uploads are allowed: after authentication, the read deadline is at least
120 s plus the time needed to transfer the maximum size at 256 KiB/s. Media
downloads extend their write deadline the same way — sized for the bytes the
response has to send at 256 KiB/s, capped at 15 minutes — so a large file is not
cut off on a slow connection (`/{name}`, `?v=`, `?i=`, `/api/preview/{name}` and
the gallery file).

### `GET /api/preview/{name}`

Returns the WebP thumbnail. For videos, or when the thumbnail is missing, it
returns the original media. Always admin-only, whatever the link's access
level. `Cache-Control: private, no-cache`: the browser keeps a copy but
revalidates it on every use.

### `GET /api/external-images`

Returns a JSON array of media paths below `EXTERNAL_IMAGE_DIR`, relative and
slash-separated, for example `["holiday/beach.jpg"]`. The listing:

- Skips hidden files and directories.
- Skips files larger than `MAX_UPLOAD_MB`.
- Does not descend deeper than `MAX_WALK_DEPTH`.
- Ignores symlinks that resolve outside the directory.
- Stops at 5,000 files.

The listing is cached for 10 seconds, so a file added to the gallery can take
up to 10 seconds to appear.

### `GET /api/external-image-preview?path=...`

Returns one gallery file after validating its type, with
`Cache-Control: private, no-cache`.

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

- Only one run at a time; a second request while one is running returns `429`
  (no `Retry-After`: the run has no predictable end).
- Runs two previews at a time. If the shared decode budget is busy, a job waits
  for it instead of failing.
- Counts towards the upload rate limit. Only `POST` spends it: a probe with
  another method gets `405` before the limiter sees it.

## Public endpoints

### `GET /{name}` (and `HEAD`)

Serves the media of a link:

- `Content-Type`, `Content-Disposition: inline`, `ETag`, `Last-Modified`.
- `Cache-Control: private, no-cache` for `public` and `local` links: browsers
  revalidate before every use, so a changed or revoked link takes effect
  immediately. `token` and `auth` links are `no-store`.
- HTTP range requests are supported.
- Cross-origin embedding is allowed (`Cross-Origin-Resource-Policy: cross-origin`).
- `Access-Control-Allow-Origin` and friends are added when `CORS_ORIGINS` is
  configured and the request's `Origin` is listed (see Conventions).

**Which file is served**

| URL | Serves |
| --- | --- |
| `/{name}` | The live file, or the playlist position the rotation clock points at |
| `/{name}?v=3` | Archived version 3. `?v=` equal to the current version serves the live file. |
| `/{name}?i=0` | The live file, ignoring rotation |
| `/{name}?i=2` | The playlist item with id 2 |
| `/{name}.jpg`, `/{name}.png`, `/{name}/latest`, `/{name}.jpg/latest` | Aliases for `/{name}` |

- The **stored** extension decides `Content-Type` and `Content-Disposition`,
  never the alias in the URL: `/frame.jpg` on a PNG link still serves
  `image/png`.
- Only extensions from the supported media list are treated as aliases, so
  `/manifest.json`, `/favicon.ico` and `/sw.js` keep answering `404` as before.
  `/robots.txt` is the one reserved name that answers something: see below.
- An unusable selector is a `404`, not a fallback: a display pinned to `?v=3`
  must never silently start showing something else.
- Rotation is derived from `floor(now / interval)` (sequential) or a stable hash
  of the link name and that window (random), so it needs no worker and no state,
  and every replica reading the same `data/` directory agrees.

**Access counters.** Every delivered response (`200`, `206`, `304`, `HEAD`)
increments the link's in-memory `stats`; refused requests do not. Counting is
done in the response writer's `ReadFrom` path, so media still uses `sendfile`.

| Access level | Requirement | Denied |
| --- | --- | --- |
| `public` | none | — |
| `local` | Client IP is loopback, RFC 1918, link-local, CGNAT or IPv6 ULA | `403` |
| `token` | `?token=…` or `X-Access-Token: …`; admin Basic Auth also works | `403` |
| `auth` | Admin session or preemptive Basic Auth from non-browser clients (no browser challenge) | `401` (`403` with `DISABLE_AUTH=true`) |

- A link without media returns `404`.
- Wrong admin credentials count towards the login lockout.

### `GET /health`

Liveness check, always public, no disk I/O. Returns
`{"service":"lanpaper","status":"ok"}`. The release version is deliberately not
reported, so an unauthenticated probe cannot tell which release to attack. Any method other than
`GET`/`HEAD` is `405`.

### `GET /health/ready`

Readiness check. It verifies that `data/` and `data/media/` are accessible
and that at least 1 GB of disk space is free. Any method other than
`GET`/`HEAD` is `405`.

- Ready: `200` with `{"status":"ready","checks":{...}}`.
- Not ready: `503`, with a `message` on each failing check.

### `GET /robots.txt`

Always `200` with `User-agent: *` / `Disallow: /`, cached for an hour. Every
path on this service is either an admin route or a mutable media link whose
bytes change without the URL changing, so crawling it costs bandwidth and
indexes content that is stale by design. The name is reserved, so no link can
be shadowed by it. An operator who wants a different policy can override this
path at the reverse proxy.

### Other routes

| Route | Behaviour |
| --- | --- |
| `GET /admin` | Admin panel. Without a session it shows the sign-in form. |
| `POST /api/session` | Validate credentials and create a browser session. |
| `DELETE /api/session` | Durably revoke the session in the request cookie; idempotent. |
| `GET /api/sessions` | List opaque ID, creation, expiry and current marker for active browser sessions; never tokens, digests, IPs or user agents. |
| `DELETE /api/sessions` | Authenticated operation that durably revokes every browser session. |
| `/admin.html` | Permanent redirect to `/admin`. |
| `/` | Redirect to `/admin`. |
| `GET /sw.js` | Service worker (root scope). |
| `GET /static/...` | Allowlisted application assets only. Legacy `static/images/` is never served. |
