# Lanpaper remediation roadmap

**Created:** 2026-10-09  
**Source audit:** [`DEEP_AUDIT_2026-10-09.md`](DEEP_AUDIT_2026-10-09.md)

This roadmap orders changes by security risk, dependency, compatibility and reviewability. “PR” below means one independently reviewable change set. Later PRs must be rebased on the accepted earlier ones; storage and frontend migrations deliberately avoid a big-bang rewrite.

## Delivery rules

Every PR must:

- preserve fail-closed behaviour;
- include tests for the changed invariant;
- pass formatting, vet, race, unit, frontend, E2E and vulnerability checks where applicable;
- update SECURITY/README/API/CHANGELOG when behaviour changes;
- include rollback and migration notes for persistent formats;
- avoid combining behavioural changes with unrelated formatting.

## Phase 0 — immediate authentication correctness

### PR 1 — Durable single-session logout **(implemented; full Go CI pending)**

**Goal:** never report successful logout unless revocation is persisted.

Changes:

- make session revocation transactional in memory and on disk;
- restore the in-memory session if persistence fails;
- return HTTP 500 and retain the cookie on failure so logout can be retried;
- redirect to the login page only after HTTP 204;
- localize the failure message;
- inject persistence failure in tests and verify retry plus restart durability.

Acceptance:

- failed persistence cannot produce false success;
- successful retry survives simulated restart;
- ordinary and idempotent logout still return 204;
- desktop and mobile E2E remain green.

### PR 2 — Revoke all sessions **(implemented; full Go CI pending)**

Added authenticated `DELETE /api/sessions`, atomic rollback on persistence
failure, “Sign out everywhere” UI with confirmation, CSRF-protected routing,
restart/integration tests and no IP/User-Agent storage.

### PR 3 — Session metadata and lifecycle hygiene **(implemented; full Go CI pending)**

Added opaque session IDs, creation/expiry timestamps, authenticated lifecycle
listing, current-session indication and automatic migration from the original
digest/expiry schema without IP or user-agent collection.

### PR 4 — Separate browser sessions from Basic Auth on protected media **(implemented; full Go CI pending)**

Browser sessions now authorize `auth` media without a Basic challenge. Machine
clients retain preemptive Basic Auth, while missing credentials return 401
without populating the browser's origin-wide Basic credential cache.

**Retrospective 1–4 completed:** [`RETROSPECTIVE_PR1_PR4.md`](RETROSPECTIVE_PR1_PR4.md).
It found and corrected legacy-ID normalization, stale startup cleanup and the
remaining cached-Basic fallback on browser API requests.

### PR 5 — Hashed administrator credentials **(implemented and CI-verified)**

Added `ADMIN_PASSWORD_HASH` using bounded Argon2id PHC verification, a
standard-input hash-generation command, password-rotation session invalidation
and deprecation warnings for plaintext `adminPass`. `ADMIN_PASS` remains a
documented migration path for one compatibility cycle.

### PR 6 — Split insecure TLS controls **(implemented and CI-verified)**

Added independent `REMOTE_INSECURE_SKIP_VERIFY` and
`PROXY_INSECURE_SKIP_VERIFY` controls, retained the broad switch as a deprecated
compatibility alias, and tested that target opt-out cannot weaken proxy TLS.

## Phase 1 — recovery, testing and observability

### PR 7 — Persistence fault-injection framework **(implemented and CI-verified)**

Introduced a shared durable atomic-write primitive with deterministic seams for
create, chmod, write, file sync/close, rename and directory open/sync/close.
Session and wallpaper metadata persistence use it, with a fault-matrix test that
does not rely on filesystem permissions.

### PR 8 — Upload transaction fault matrix **(implemented and CI-verified)**

Made `validate → stage → publish → commit → finalize/rollback` an explicit
state machine around the existing atomic media publication. Fault-matrix tests
cover first/second publish failure, metadata-commit failure, reverse-order
rollback, successful finalization and rejected out-of-order transitions; staged
request files remain cleanup-owned until publication.

**Retrospective 5–8 completed:** [`RETROSPECTIVE_PR5_PR8.md`](RETROSPECTIVE_PR5_PR8.md).
It found and corrected post-rename durability errors that could otherwise leave
memory behind the already-renamed on-disk state.

### PR 9 — Read-only storage audit command **(implemented and CI-verified)**

Added `lanpaper audit [--json] [--root DIR]` reporting missing referenced files,
orphans, stale temporary files, invalid types/permissions and history/playlist
size drift. Audits use a read-only filesystem walk and return a nonzero status
when inconsistencies are found.

### PR 10 — Explicit repair command **(implemented and CI-verified)**

Added explicit `lanpaper repair --dry-run|--apply`, timestamped quarantine,
fsynced JSONL operation journal, metadata backup, exclusive repair lock and
recovery tests. Missing live media and invalid records remain manual rather
than being guessed or deleted.

### PR 11 — Structured logging **(implemented and CI-verified)**

Adopted console-readable `log/slog` output with stable event fields, an adapter
for legacy call sites, named authentication events and centralized redaction of
password, token, cookie, authorization, proxy-secret, URL and query attributes.
Redaction and compatibility output are covered by tests.

### PR 12 — Operational metrics **(implemented and CI-verified)**

Added the opt-in, administrator-authenticated `/metrics` endpoint with bounded
Prometheus status-class and latency-bucket counters, active uploads, bounded
processing queue, rate-limit rejections, active sessions, persistence failures
and data-directory usage. No paths, link names, clients or tokens are labels.

**Retrospective 9–12 completed:** [`RETROSPECTIVE_PR9_PR12.md`](RETROSPECTIVE_PR9_PR12.md).
It corrected ignored audit traversal failures, non-durable repair metadata
replacement and authentication disclosure on the disabled metrics endpoint.

### PR 13 — Broader browser E2E **(implemented and CI-verified)**

Added desktop/phone browser workflows covering create, repeated upload, rename,
access changes, token rotation, history, playlist append, token-safe link-list
export/import semantics and delete. Multi-tab tests verify that logout expires
other tabs without a cached Basic-auth fallback; persistence rollback remains
covered by deterministic integration tests.

### PR 14 — Accessibility and cross-browser CI **(implemented and CI-verified)**

Added axe serious/critical checks, keyboard-only create-dialog focus checks,
200% zoom at a 320 px viewport, long German settings labels and a Desktop
Safari/WebKit project. CI installs and runs both Chromium and WebKit; focus
restoration and existing live-region semantics are asserted directly.

### PR 15 — Security fuzzing **(implemented and CI-verified)**

Added bounded, regression-seeded Go fuzz targets for multipart fields and file
bytes, route selectors, Origin/Forwarded/X-Forwarded-For handling and security
middleware, plus wallpaper/history/playlist metadata decoding and
normalization. Oversized fuzz inputs are rejected before expensive work; normal
Go CI executes every seed corpus.

## Phase 2 — resource governance and backend modularity

### PR 16 — Rate-limiter model upgrade **(implemented and CI-verified)**

Replaced fixed windows with sharded continuously refilled token buckets whose
capacity includes the configured burst while refill uses the sustained rate.
Login, publish-key failure, upload/regeneration and public download namespaces
remain independent; the existing global upload/decode ceilings bound heavy
work. Startup now warns when public or upload limits are disabled. State remains
explicitly single-instance and in-memory.

**Retrospective 13–16 completed:** [`RETROSPECTIVE_PR13_PR16.md`](RETROSPECTIVE_PR13_PR16.md).
It corrected non-representative export/import coverage, a fuzz compilation
error and accessibility/viewport assumptions, and added missing session/media
fuzz coverage.

### PR 17 — Application composition root **(implemented and CI-verified)**

Added `App` with copied immutable configuration, wallpaper services, session and
limiter lifecycle interfaces and structured logger. The complete production
route/middleware stack is now built by `App.Handler()`; `newHandler` is a narrow
compatibility adapter while subsequent PRs replace package globals.

### PR 18 — Session and limiter instance isolation **(partially implemented; request injection remains)**

Introduced independently constructible `SessionStore` and `RateStore` owners,
with instance-scoped session counting, token admission, cleanup and reset;
isolation tests prove their mutable maps do not cross instances. The 1–20
retrospective found that request middleware still consumes the process runtime,
so `NewApp` now truthfully references that runtime rather than advertising
unused fresh stores. Context/closure injection through every auth and limiter
handler remains required before multiple isolated Apps are supported.

### PR 19 — Upload service extraction **(implemented and CI-verified)**

Extracted `UploadService` with injected wallpaper storage as the owner of source
resolution, inspection, processing, publication and metadata commit. The App
route uses the service directly while the old handler name is a compatibility
adapter. Typed stage errors preserve underlying causes and the existing HTTP
response contract.

### PR 20 — History and playlist service extraction **(implemented and CI-verified)**

Added injected `LibraryService` for history listing/rollback/deletion and
playlist metadata/file removal. Store-scoped history deletion preserves byte
accounting; playlist bytes are removed only after metadata commit. App routes
use the service directly, typed errors retain not-found causes, and compatibility
entry points preserve the API contract.

**Retrospective 17–20 completed:** [`RETROSPECTIVE_PR17_PR20.md`](RETROSPECTIVE_PR17_PR20.md).
It corrected compatibility services that captured a stale replaceable global
store and revalidated transaction, counter and error boundaries.

**Comprehensive PR 1–20 retrospective:** [`RETROSPECTIVE_PR1_PR20.md`](RETROSPECTIVE_PR1_PR20.md).
It corrected an inaccurate per-App runtime ownership claim, marked remaining
request-level isolation work explicitly, and bounded metrics disk-scan cost.

### PR 21 — Remote fetcher interface **(implemented and CI-verified)**

Added `RemoteFetcher` with injected public-URL resolver, round tripper, clock and
temporary directory while retaining per-hop IP pinning, SNI/Host preservation,
redirect scheme/count checks and independent target/proxy TLS policy. Upload
services own their fetcher. Remote-download and media-processing concurrency now
use separate bounded semaphores so slow networks do not consume decode slots.

## Phase 3 — frontend maintainability and UX

### PR 22 — Frontend API/error module **(implemented and CI-verified)**

Extracted fetch/serialization/decoding into native `api.js` with typed
`ApiError`, cancellation, retryability, optional `X-Error-Code` machine codes
and centralized session-expiry handling. The panel and export/import scripts now
load as ordered native modules; legacy server text remains only as a display
fallback where endpoints do not yet emit codes.

### PR 23 — Frontend state and rendering modules **(implemented and CI-verified)**

Extracted state defaults, API-record normalization, filters, selectors, sorting
and incremental-render transitions into the framework-free `state.js` module.
The existing plain-DOM card renderer consumes those selectors, while focused
state-transition tests cover combined filtering, pin ordering and render resets.

### PR 24 — Feature modules **(implemented and CI-verified)**

Added a module-local feature registry and immutable capability facade. Link-list
export/import is a registered feature and no longer depends on the broad
`window.LanpaperApp` object or mutable panel state. Upload orchestration now
lives in an injected controller; access, history, playlist and settings/export
models are isolated in `feature-domain.js`. `app.js` retains DOM composition,
while dialog lifecycle moves next to the dedicated overlay controller in PR 25.

**Retrospective 21–24 completed:** [`RETROSPECTIVE_PR21_PR24.md`](RETROSPECTIVE_PR21_PR24.md).

### PR 25 — Accessible overlay controller **(implemented and CI-verified)**

Centralized nested dialog stacking, background inertness, focus trap/restore,
scroll lock, Escape/scrim dismissal and live-region announcements in an
injected controller. Visual-viewport resize handling keeps the focused field
visible above mobile virtual keyboards; focused tests cover nesting and Escape.

### PR 26 — Bulk import API and progress UI **(implemented and CI-verified)**

Added an admin-only validate-first/dry-run API with atomic batches capped at
100 records, machine-readable validation codes and per-record results. The UI
dry-runs every batch before mutation, reports progress, supports cancellation
between batches and downloads a combined report. Link-list exports never carry
secrets; imported token links receive fresh tokens. UI and documentation now
consistently distinguish a link-list export from a media backup.

### PR 27 — Offline and retry UX **(implemented and CI-verified)**

Added an announced online/offline banner, explicit reconnect action, retry
capabilities for recoverable uploads, keyed duplicate-submission guards and
long-operation progress. The service worker cache generation was bumped while
its network-only admin/API/media policy remains enforced and contract-tested.

## Phase 4 — measured storage evolution

### PR 28 — Storage scale benchmarks and supported limits **(implemented and CI-verified)**

Added reproducible 1k/10k/50k benchmarks for list/create/update/rename/delete,
JSON serialization and durable atomic persistence. Published the raw runner
results and an evidence-based 10k supported ceiling, 10k–50k transitional
range, 50k hard migration boundary and latency/file-size migration triggers in
[`STORAGE_SCALE_BENCHMARK.md`](STORAGE_SCALE_BENCHMARK.md).

**Retrospective 25–28 completed:** [`RETROSPECTIVE_PR25_PR28.md`](RETROSPECTIVE_PR25_PR28.md).

### PR 29 — `WallpaperStore` abstraction **(implemented and CI-verified)**

Introduced context-aware CRUD/list and atomic transaction interfaces, with the
existing JSON store retained behind `JSONWallpaperStore`. The application
composition root now exposes the interface. Shared conformance tests cover
independent reads, CRUD, cancellation, rollback, multi-record commit and the
single-durable-write transaction guarantee.

### PR 30 — SQLite schema and implementation **(implemented; full CI pending)**

Added the pure-Go `SQLiteWallpaperStore` with normalized wallpaper, history,
playlist and rotation tables; strict checks, foreign keys, WAL, full-sync
transactions and the shared store conformance contract. Integrity checking and
online `VACUUM INTO` backup primitives are included. Media bytes remain in the
existing filesystem layout.

### PR 31 — JSON-to-SQLite migration **(implemented; full CI pending)**

Added the explicit `migrate-sqlite` command with validation-only dry runs,
SHA-256 source identity, fsynced and verified backup-first operation,
deterministic bounded transactions, durable resumable checkpoints, integrity
verification and rollback. JSON is never modified or deleted. Operational,
interruption and downgrade guidance is in
[`SQLITE_MIGRATION.md`](SQLITE_MIGRATION.md).

### PR 32 — Server-side pagination/filtering **(implemented; full CI pending)**

Expanded the paginated envelope with validated access/kind/search filters and
created/updated/name/size sorting. The modular frontend loads bounded 200-record
pages while retaining its local selectors and current UX. Legacy clients keep
the bare array contract, bounded to 10,000 records with explicit truncation
headers.

**Retrospective 29–32 completed:** [`RETROSPECTIVE_PR29_PR32.md`](RETROSPECTIVE_PR29_PR32.md).

### PR 33 — Background processing pool **(implemented; full CI pending)**

Added a shared bounded processing pool with independent CPU slots and decoded
pixel memory reservations, cancellation-aware waiting and observable queue,
active, completion and memory counters. Upload publication still waits for its
full durable transaction. Preview regeneration runs through bounded workers,
checks cancellation before publication/metadata commit and exposes pollable
progress consumed by the settings UI.

## Phase 5 — release and supply-chain hardening

### PR 34 — Hardened deployment profile **(implemented; full CI pending)**

The production Compose contract now binds to loopback, requires an Argon2id
credential, keeps the root filesystem read-only, bounds `/tmp`, drops all
capabilities, prevents privilege escalation and sets CPU, memory and PID limits.
Only `/app/data` remains writable. CI boots and health-checks the official image
with equivalent isolation and validates the documented Compose configuration.

### PR 35 — SBOM, provenance and image signing

Generate SPDX/CycloneDX SBOMs, publish build provenance, sign release images and document verification.

### PR 36 — Disaster-recovery runbook

Document backup scope, restore drills, audit/repair usage, password/session rotation, corrupted metadata handling, disk-full response and version rollback. Add a CI restore smoke test.

## Dependency chain

- PR 1–6 are independent of the later architecture but must land first for security semantics.
- PR 7 enables reliable PR 8–10 testing.
- PR 17–21 prepare storage and frontend APIs without requiring SQLite.
- PR 22–27 should land before pagination changes to avoid expanding the current monolith.
- PR 28 decides whether PR 30–33 are necessary for the deployment scale; interfaces still improve testability even if SQLite is deferred.
- PR 34–36 land after command-line/storage formats stabilize.

## Completion definition

The remediation programme is complete when:

- logout and global revocation are durable under injected write failures;
- browser authentication has no implicit Basic-auth fallback;
- recovery tooling can explain and safely repair every file/metadata mismatch;
- critical workflows pass desktop, phone, keyboard, accessibility and cross-browser E2E;
- resource limits cover every attacker-controlled expensive path;
- backend and frontend have explicit module boundaries;
- storage has measured limits and a tested migration path;
- releases ship verifiable artifacts and a tested recovery runbook.
