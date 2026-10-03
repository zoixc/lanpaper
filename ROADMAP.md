# Roadmap

Open ideas, roughly in priority order. For shipped changes see
[CHANGELOG.md](CHANGELOG.md).

## Features

- **Schedules.** Rotation already switches a link between its playlist items at
  an interval; what is missing is picking one by time of day or day of week
  ("news at 08:00, artwork at night"), and an optional per-item weight.
- **Signed publish requests.** `PUBLISH_KEYS` authorizes a client, not a
  payload. An optional HMAC signature over the link name and a timestamp would
  let a webhook prove that a specific push was intended, and would expire a
  replayed request.
- **Version metadata.** An optional note per archived version ("why it was
  replaced"), and a diff view of what changed between two versions.
- **Animated media.**
  - Animated WebP support.
  - Animation-preserving GIF/WebP processing in compressed mode.
  - Both need frame-count and CPU budgets.
- **More input formats.** HEIC/HEIF and AVIF conversion on upload. ICO
  (largest frame) for favicon collections. SVG only with strict sanitisation
  or rasterisation.
- **Token options.** Optional expiry dates and multiple tokens per link.
- **Persisted access statistics.** The per-link counters are in memory and reset
  on restart, which keeps them free and private. An optional rolling daily
  series on disk would survive restarts without storing query strings (tokens)
  or visitor identities.
- **Full backup and restore.** A media-inclusive archive with explicit
  conflict handling, to replace the metadata-only browser export. It has to
  cover `data/history/` and `data/items/` as well.

## Scalability and operations

- Paginate the admin UI and the gallery for very large libraries. The API
  already supports pagination.
- Put storage behind an interface (e.g. SQLite) for large libraries or
  several instances. Rotation is already stateless, so several readers of the
  same `data/` directory agree without coordination.
- Optional disk quota for `data/media/`, in addition to `MAX_IMAGES` and
  `HISTORY_MAX_MB`.
- Admin-only metrics endpoint: upload queue, rate-limit hits, decode errors,
  history budget in use.
- Storage-tier awareness for archives: an optional second directory (or a
  cheaper volume) for `data/history/`, since archived versions are read rarely.

## Security

- Hashed admin credentials (bcrypt/argon2) and optional session cookies
  instead of Basic Auth on every request.
- Per-key scoping and revocation for `PUBLISH_KEYS`: today every key may publish
  to every link, and rotating one means restarting the process.
- Optional CONNECT-only mode for outbound HTTP proxies, with proxy
  conformance tests.
- Optional sandboxed decoding or malware scanning for untrusted uploaders.

## Quality

- Browser end-to-end tests in CI (create, upload, access levels, import,
  history rollback, playlist rotation).
- Benchmarks for the public media path (`BenchmarkPublic…`), so hot-path
  changes stay measured instead of assumed. The test harness would need a
  `*testing.B` variant of `setupApp` first.
- Fuzz tests for media sniffing, path validation and URL checks.
- An arm64 runtime smoke test in CI; the Docker job currently checks amd64 only.
