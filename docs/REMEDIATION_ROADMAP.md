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

### PR 5 — Hashed administrator credentials **(implemented; full CI pending)**

Added `ADMIN_PASSWORD_HASH` using bounded Argon2id PHC verification, a
standard-input hash-generation command, password-rotation session invalidation
and deprecation warnings for plaintext `adminPass`. `ADMIN_PASS` remains a
documented migration path for one compatibility cycle.

### PR 6 — Split insecure TLS controls **(implemented; full CI pending)**

Added independent `REMOTE_INSECURE_SKIP_VERIFY` and
`PROXY_INSECURE_SKIP_VERIFY` controls, retained the broad switch as a deprecated
compatibility alias, and tested that target opt-out cannot weaken proxy TLS.

## Phase 1 — recovery, testing and observability

### PR 7 — Persistence fault-injection framework **(implemented; full CI pending)**

Introduced a shared durable atomic-write primitive with deterministic seams for
create, chmod, write, file sync/close, rename and directory open/sync/close.
Session and wallpaper metadata persistence use it, with a fault-matrix test that
does not rely on filesystem permissions.

### PR 8 — Upload transaction fault matrix **(implemented; full CI pending)**

Made `validate → stage → publish → commit → finalize/rollback` an explicit
state machine around the existing atomic media publication. Fault-matrix tests
cover first/second publish failure, metadata-commit failure, reverse-order
rollback, successful finalization and rejected out-of-order transitions; staged
request files remain cleanup-owned until publication.

**Retrospective 5–8 completed:** [`RETROSPECTIVE_PR5_PR8.md`](RETROSPECTIVE_PR5_PR8.md).
It found and corrected post-rename durability errors that could otherwise leave
memory behind the already-renamed on-disk state.

### PR 9 — Read-only storage audit command **(implemented; full CI pending)**

Added `lanpaper audit [--json] [--root DIR]` reporting missing referenced files,
orphans, stale temporary files, invalid types/permissions and history/playlist
size drift. Audits use a read-only filesystem walk and return a nonzero status
when inconsistencies are found.

### PR 10 — Explicit repair command **(implemented; full CI pending)**

Added explicit `lanpaper repair --dry-run|--apply`, timestamped quarantine,
fsynced JSONL operation journal, metadata backup, exclusive repair lock and
recovery tests. Missing live media and invalid records remain manual rather
than being guessed or deleted.

### PR 11 — Structured logging **(implemented; full CI pending)**

Adopted console-readable `log/slog` output with stable event fields, an adapter
for legacy call sites, named authentication events and centralized redaction of
password, token, cookie, authorization, proxy-secret, URL and query attributes.
Redaction and compatibility output are covered by tests.

### PR 12 — Operational metrics **(implemented; full CI pending)**

Added the opt-in, administrator-authenticated `/metrics` endpoint with bounded
Prometheus status-class and latency-bucket counters, active uploads, bounded
processing queue, rate-limit rejections, active sessions, persistence failures
and data-directory usage. No paths, link names, clients or tokens are labels.

**Retrospective 9–12 completed:** [`RETROSPECTIVE_PR9_PR12.md`](RETROSPECTIVE_PR9_PR12.md).
It corrected ignored audit traversal failures, non-durable repair metadata
replacement and authentication disclosure on the disabled metrics endpoint.

### PR 13 — Broader browser E2E **(implemented; full CI pending)**

Added desktop/phone browser workflows covering create, repeated upload, rename,
access changes, token rotation, history, playlist append, token-safe link-list
export/import semantics and delete. Multi-tab tests verify that logout expires
other tabs without a cached Basic-auth fallback; persistence rollback remains
covered by deterministic integration tests.

### PR 14 — Accessibility and cross-browser CI **(implemented; full CI pending)**

Added axe serious/critical checks, keyboard-only create-dialog focus checks,
200% zoom at a 320 px viewport, long German settings labels and a Desktop
Safari/WebKit project. CI installs and runs both Chromium and WebKit; focus
restoration and existing live-region semantics are asserted directly.

### PR 15 — Security fuzzing **(implemented; full CI pending)**

Added bounded, regression-seeded Go fuzz targets for multipart fields and file
bytes, route selectors, Origin/Forwarded/X-Forwarded-For handling and security
middleware, plus wallpaper/history/playlist metadata decoding and
normalization. Oversized fuzz inputs are rejected before expensive work; normal
Go CI executes every seed corpus.

## Phase 2 — resource governance and backend modularity

### PR 16 — Rate-limiter model upgrade **(implemented; full CI pending)**

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

### PR 17 — Application composition root **(implemented; full CI pending)**

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

### PR 19 — Upload service extraction **(implemented; full CI pending)**

Extracted `UploadService` with injected wallpaper storage as the owner of source
resolution, inspection, processing, publication and metadata commit. The App
route uses the service directly while the old handler name is a compatibility
adapter. Typed stage errors preserve underlying causes and the existing HTTP
response contract.

### PR 20 — History and playlist service extraction **(implemented; full CI pending)**

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

### PR 21 — Remote fetcher interface **(implemented; full CI pending)**

Added `RemoteFetcher` with injected public-URL resolver, round tripper, clock and
temporary directory while retaining per-hop IP pinning, SNI/Host preservation,
redirect scheme/count checks and independent target/proxy TLS policy. Upload
services own their fetcher. Remote-download and media-processing concurrency now
use separate bounded semaphores so slow networks do not consume decode slots.

## Phase 3 — frontend maintainability and UX

### PR 22 — Frontend API/error module

Extract API calls, typed errors, cancellation and authentication-expiry handling into a native ES module. Remove server-English regex matching where machine-readable error codes can be introduced compatibly.

### PR 23 — Frontend state and rendering modules

Extract state/selectors, cards, filters and incremental rendering. Preserve plain DOM and no runtime framework. Add DOM-level tests for state transitions.

### PR 24 — Feature modules

Split upload, access, history, playlist, settings and dialogs. Reduce the root `app.js` to composition/startup and remove the broad `window.LanpaperApp` surface in favour of a narrow facade.

### PR 25 — Accessible overlay controller

Centralize dialog stacking, focus trap/restore, scroll lock, Escape handling and live-region announcements. Verify nested dialogs and mobile virtual keyboard behaviour.

### PR 26 — Bulk import API and progress UI

Add validate-first/dry-run bulk import with bounded transaction batches, progress, cancellation and per-record report. Keep access tokens excluded. Rename UI language consistently from full “backup” to “link-list export”.

### PR 27 — Offline and retry UX

Add online/offline state, retry actions for recoverable operations, duplicate-submission guards and operation progress without caching admin/API responses in the service worker.

## Phase 4 — measured storage evolution

### PR 28 — Storage scale benchmarks and supported limits

Benchmark 1k/10k/50k records for list/create/update/rename/delete, allocations and persistence latency. Publish an evidence-based supported range and migration trigger.

### PR 29 — `WallpaperStore` abstraction

Introduce context-aware CRUD/list transaction interfaces while retaining the JSON implementation. Add conformance tests shared by all implementations.

### PR 30 — SQLite schema and implementation

Add normalized metadata/history/playlist schema, WAL, transactions, integrity constraints and backup primitives. Keep media bytes on the filesystem.

### PR 31 — JSON-to-SQLite migration

Implement resumable, checksummed, backup-first migration with dry-run, rollback and downgrade documentation. Never delete the original JSON automatically.

### PR 32 — Server-side pagination/filtering

Add backward-compatible pagination, filtering and sorting; update the modular frontend. Preserve a bounded compatibility response for older clients.

### PR 33 — Background processing pool

Move decode/preview/regeneration work into bounded jobs with cancellation, progress and independent CPU/memory budgets. Do not acknowledge publication before its required durable state exists.

## Phase 5 — release and supply-chain hardening

### PR 34 — Hardened deployment profile

Add read-only-root, no-new-privileges, dropped capabilities, explicit writable mounts and resource-limit examples. Test the official image under that profile.

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
