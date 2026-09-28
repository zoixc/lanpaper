# Roadmap

Open ideas, roughly in priority order. Nothing here is implemented yet. For
shipped changes see [CHANGELOG.md](CHANGELOG.md).

## Features

- **Playlists and schedules.** A link rotates through several images at an
  interval, or picks one by time of day or day of week.
- **Animated media.**
  - Animated WebP support.
  - Animation-preserving GIF/WebP processing in compressed mode.
  - Both need frame-count and CPU budgets.
- **More input formats.** HEIC/HEIF and AVIF conversion on upload. SVG only
  with strict sanitisation or rasterisation.
- **Token options.** Optional expiry dates and multiple tokens per link.
- **Access statistics.** Per-link request counts, stored without query
  strings (tokens) and with bounded retention.
- **Full backup and restore.** A media-inclusive archive with explicit
  conflict handling, to replace the metadata-only browser export.

## Scalability and operations

- Paginate the admin UI and the gallery for very large libraries. The API
  already supports pagination.
- Put storage behind an interface (e.g. SQLite) for large libraries or
  several instances.
- Optional disk quota for `data/media/`, in addition to `MAX_IMAGES`.
- Admin-only metrics endpoint: upload queue, rate-limit hits, decode errors.

## Security

- Hashed admin credentials (bcrypt/argon2) and optional session cookies
  instead of Basic Auth on every request.
- Optional CONNECT-only mode for outbound HTTP proxies, with proxy
  conformance tests.
- Optional sandboxed decoding or malware scanning for untrusted uploaders.

## Quality

- Browser end-to-end tests in CI (create, upload, access levels, import).
- Fuzz tests for media sniffing, path validation and URL checks.
- An arm64 runtime smoke test in CI; the Docker job currently checks amd64 only.
