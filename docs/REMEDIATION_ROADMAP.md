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

### PR 5 — Hashed administrator credentials

Add `ADMIN_PASSWORD_HASH` using Argon2id, a hash-generation command, dummy verification, password-rotation session invalidation and deprecation warning for plaintext `adminPass` in JSON. Preserve `ADMIN_PASS` as a documented migration path for one compatibility cycle.

### PR 6 — Split insecure TLS controls

Replace the broad switch with `REMOTE_INSECURE_SKIP_VERIFY` and `PROXY_INSECURE_SKIP_VERIFY`; keep a temporary deprecated alias. Test target TLS and HTTPS-proxy TLS independently.

## Phase 1 — recovery, testing and observability

### PR 7 — Persistence fault-injection framework

Introduce test seams for write, sync, close, rename and directory sync. Cover disk-full, read-only, interrupted session writes and metadata commit failures without relying on filesystem permissions.

### PR 8 — Upload transaction fault matrix

Model `validate → stage → publish → commit → finalize/rollback`. Add tests for failure at every boundary, client disconnect and process interruption. No functional redesign yet.

### PR 9 — Read-only storage audit command

Add `lanpaper audit` reporting missing files, orphan files, stale temporary files, invalid types/permissions, history drift and playlist drift. Produce human-readable and JSON output; never mutate.

### PR 10 — Explicit repair command

Add `lanpaper repair --dry-run` and `--apply`, quarantine rather than immediate deletion, operation journal and recovery tests. Keep repair offline or under an exclusive application lock.

### PR 11 — Structured logging

Adopt `log/slog`, stable event names and redaction tests. Never log query strings, tokens, cookies, passwords or proxy credentials. Keep console-readable defaults.

### PR 12 — Operational metrics

Add an optional authenticated metrics endpoint for request latency/status, active uploads, processing queue, rate-limit rejection, session count, persistence failures and disk usage. Document privacy properties and cardinality bounds.

### PR 13 — Broader browser E2E

Cover create, upload, rename, access changes, token rotation, history, playlist, import/export and delete on desktop and phone. Add multi-tab session expiration and logout persistence-error scenarios.

### PR 14 — Accessibility and cross-browser CI

Add axe checks, keyboard-only flows, zoom/narrow viewport cases, long translations and WebKit. Fix focus restoration, live regions or contrast defects found by those tests in narrowly scoped follow-ups.

### PR 15 — Security fuzzing

Add fuzz targets for multipart parsing, URL/Origin/forwarded headers, route selectors, media signatures, metadata/session decoding and CORS normalization. Seed with regression fixtures and enforce bounded allocations.

## Phase 2 — resource governance and backend modularity

### PR 16 — Rate-limiter model upgrade

Move fixed windows to token buckets, separate login/publish/upload/download/regeneration budgets, add a global heavy-work ceiling and warnings for unsafe disabled limits. Preserve single-instance semantics explicitly.

### PR 17 — Application composition root

Introduce an `App` struct holding immutable config, stores, sessions, limiters, logger and services. Build routes through `App.Handler()`. Migrate globals incrementally with compatibility adapters.

### PR 18 — Session and limiter instance isolation

Move the remaining global session/rate state into injected stores. Remove test reset globals and allow multiple isolated app instances in one process.

### PR 19 — Upload service extraction

Extract source resolution, inspection, image processing and publish transaction from the HTTP handler. Add typed errors and preserve the existing API response contract.

### PR 20 — History and playlist service extraction

Encapsulate history byte accounting and playlist mutations so handlers cannot directly violate counters or file/metadata invariants.

### PR 21 — Remote fetcher interface

Inject resolver, transport and clock; retain IP pinning and per-redirect checks. Improve proxy tests, timeout tests and separate processing concurrency from network concurrency.

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
