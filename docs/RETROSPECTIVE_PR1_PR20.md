# Comprehensive retrospective: roadmap PR 1–20

**Date:** 2026-10-10  
**Range reviewed:** `f52ad51` through `6ad35de`

## Executive conclusion

The production single-instance deployment remains functionally compatible and
passes stable/oldstable Go, race, vet, frontend, Chromium/phone/WebKit E2E and
Docker smoke checks. Authentication, persistence and upload failure semantics
are materially safer than at the start of the programme. No credential, token,
cookie or query-string disclosure was found in current logging or metrics.

The review did find one architectural overstatement and one scale regression:

1. `App` contained fresh session/limiter stores, but compatibility middleware
   still consumed the process stores. The fields were not the runtime actually
   serving requests, so multiple isolated Apps were not truly supported.
2. Every authenticated metrics scrape recursively walked `data/`, making scrape
   cost proportional to the entire library.

Both are corrected in the accompanying retrospective fix: `App` now references
the runtime actually consumed by current middleware, PR 18 is explicitly marked
partial until request-level injection is complete, and disk usage is cached for
30 seconds. No unsupported isolation guarantee is now presented to operators or
developers.

## Review by boundary

### 1. Authentication and sessions — PR 1–5

Verified:

- failed logout persistence restores the in-memory session and does not clear
  the cookie or report false success;
- revoke-all is atomic and rollback-safe;
- persisted sessions contain token digests and opaque metadata, not bearer
  tokens, IP addresses or user agents;
- browser requests cannot fall back to cached Basic credentials after logout;
- machine clients retain explicit preemptive Basic support;
- Argon2id PHC parsing bounds memory, iterations, parallelism, salt and output
  before allocation;
- a configured hash takes precedence over deprecated plaintext;
- credential fingerprints invalidate migrated sessions after rotation;
- authentication failure namespaces remain independent token buckets.

Compatibility/performance impact:

- plaintext migration mode intentionally performs a dummy Argon2id operation;
  login is therefore more expensive but resists storage-mode timing disclosure;
- old session files receive one compatibility migration before fingerprints are
  required;
- `ADMIN_PASS` remains functional but emits a deprecation warning.

No new authentication bypass or session resurrection path was found.

### 2. Outbound TLS and remote media — PR 6

Verified target-media and HTTPS-proxy TLS controls are independent. The legacy
broad alias is applied first and each specific environment variable can override
only its side. Tests cover both directions, so trusting a self-signed origin
cannot silently trust a proxy, or vice versa.

### 3. Persistence and upload transactions — PR 7–8

Verified the atomic writer distinguishes failures before rename from
post-rename durability uncertainty. Callers do not leave old memory behind a
new on-disk file. Fault tests cover create, chmod, write, file sync/close,
rename and directory open/sync/close.

Upload ordering remains:

`validate → stage → publish → metadata commit → finalize`, with reverse-order
rollback before commit. Metadata failure cannot leave new media published, and
post-commit cleanup cannot roll back a successful operation.

### 4. Audit and repair — PR 9–10

Verified:

- audit is read-only and rejects unsafe metadata-derived names/extensions before
  constructing paths;
- unreadable traversal fails the audit instead of reporting a false clean state;
- repair requires exactly one of dry-run/apply and an exclusive lock;
- files are quarantined rather than deleted;
- metadata is backed up and atomically replaced;
- every operation and metadata commit is written to a synced JSONL journal;
- missing live media and structurally invalid records remain manual recovery
  cases rather than unsafe guesses.

A stale lock still requires operator inspection by design; automatic lock
stealing would be less safe.

### 5. Logging and metrics — PR 11–12

Structured attributes redact password/token/cookie/authorization/proxy-secret
keys. Legacy messages were searched for request queries and credentials; none
currently logs them. Metrics use fixed status classes and latency buckets, not
paths, links, clients or secrets. Disabled metrics return 404 before an auth
challenge and enabled metrics remain administrator-authenticated.

Performance correction: disk usage now has a 30-second cache, bounding normal
scrape overhead instead of walking every stored file on every scrape.

### 6. Browser, accessibility and fuzzing — PR 13–15

The browser matrix covers Chromium desktop, phone and WebKit. It exercises the
actual settings export/import module, verifies both historical and current
access tokens are absent, and verifies multi-tab session expiry. Axe findings
for the hidden picker and light-theme contrast were fixed. Keyboard focus,
focus restoration, narrow viewport and 200% zoom are covered.

Fuzz targets compile and their regression seeds run in normal CI for multipart,
media signatures, routes, forwarding/origin headers, sessions and media
metadata. Inputs are size-bounded before expensive parsing. Extended randomized
campaigns remain suitable for scheduled/release CI.

### 7. Token buckets — PR 16

Capacity and sustained refill are separate, `Retry-After` describes the next
token, IPv6 remains grouped by /64, and idle state is cleaned. Public,
upload/regeneration, login and publish-key failure namespaces are separate.
Existing upload/decode semaphores remain the process-wide heavy-work ceilings.
Disabled limits produce explicit warnings.

### 8. Composition and services — PR 17–20

Upload and history routes use injected App-owned services. Upload, history and
playlist operations preserve their existing HTTP response shapes and commit
ordering. Compatibility handlers resolve replaceable stores at request time,
which prevents stale test stores and cross-test races.

Important corrected limitation: configuration and some older handlers still use
package compatibility globals, and authentication middleware still uses the
single process runtime associated with the one supported data root. Independent
`SessionStore`/`RateStore` values are isolation-tested, but request-level
injection is not complete. PR 18 is therefore marked partial rather than claiming
multiple isolated production Apps.

## Concurrency and performance

- wallpaper readers do not wait for fsync; writers serialize copy/write/publish;
- session and limiter maps are lock-protected;
- limiter state is sharded and idle-cleaned;
- media processing remains bounded by upload and decoded-pixel ceilings;
- E2E remains serial intentionally because one server process shares brute-force
  budgets and persistent files;
- no new unbounded label cardinality, goroutine creation or attacker-controlled
  allocation was found;
- the metrics full-tree walk was the only new scale regression found and is now
  cached.

## Compatibility and rollback

Public API response formats, link URLs, media selectors and machine Basic Auth
remain compatible. Intentional impacts are documented: browser APIs require
sessions, plaintext credentials and broad TLS opt-out are deprecated, metrics
are opt-in, and repair is offline/explicit. Persistent format migrations retain
old session compatibility and never automatically delete original media or
metadata backups.

## Validation status

The baseline immediately before this retrospective fix is fully green:

- Go stable and oldstable formatting, vet, race tests and build;
- vulnerability scan on stable;
- frontend syntax and unit tests;
- Chromium desktop, phone and WebKit E2E;
- amd64 Docker build and smoke test using rate-limit-independent CI base mirrors.

The retrospective fix adds focused tests for runtime-store truthfulness and disk
usage caching and must pass the same matrix before roadmap work resumes.

## Remaining risks and required follow-up

1. Complete request-level session/limiter/config injection before claiming true
   multi-App isolation; PR 18 remains open in this respect.
2. Continue removing direct `storage.Global` use as services are extracted.
3. Run longer fuzz campaigns outside per-commit CI.
4. Exercise audit/repair against production-sized snapshots during disaster
   recovery drills.
5. Keep plaintext credential and broad TLS alias removal tied to a documented
   compatibility release rather than removing them silently.
