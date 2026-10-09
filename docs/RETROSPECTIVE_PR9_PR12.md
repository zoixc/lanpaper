# Retrospective: roadmap PR 9–12

**Date:** 2026-10-09

## Scope

Reviewed offline audit/repair safety, filesystem and crash boundaries,
structured-log privacy, metrics authentication/cardinality, API behavior and CI
coverage introduced by PR 9–12.

## Findings corrected before PR 13

### Audit traversal errors were ignored

The first audit walker discarded `WalkDir` errors, allowing an unreadable
storage subtree to look clean. Missing optional directories remain valid, but
all other traversal failures now abort the audit with exit status 2.

### Repair metadata replacement was not crash durable

Quarantine operations were journaled and synced, but repaired metadata used a
plain write/rename. Metadata backup and replacement now use the shared durable
atomic writer, and the journal records a distinct metadata commit operation.
Post-rename durability uncertainty follows the established committed-error
semantics.

### Disabled metrics endpoint exposed an authentication challenge

Authentication originally wrapped the feature-disabled check. A disabled
endpoint could therefore return 401 and advertise its Basic realm. Feature
detection now runs first and returns an ordinary 404 without an authentication
challenge; enabled metrics remain administrator-authenticated.

## Boundary review

- Audit validates link names and extensions before deriving paths and never
  follows metadata outside the configured root.
- Repair requires an explicit mode, an exclusive lock and offline operation;
  destructive guesses for missing live media remain forbidden.
- Quarantined bytes and pre-repair metadata remain recoverable. Journal records
  are flushed and synced after every state transition.
- Structured attributes centrally redact credential-like keys. Existing text
  logs are bridged with a stable `legacy_log` event while migration continues.
- Metrics labels are fixed status classes and latency buckets. No URL, link,
  client or credential value becomes a label.
- The metrics response preserves wrapped `ResponseWriter` capabilities through
  `Unwrap`, avoiding regressions in streaming and response controllers.

## Residual risks

- A stale repair lock requires operator inspection and manual removal; automatic
  lock stealing could corrupt an active repair and is intentionally absent.
- Missing previews require regeneration rather than metadata deletion, while
  missing live media requires restore from backup.
- Legacy log messages are structured as a whole message rather than individual
  fields; no current legacy call logs query strings, cookies or credentials.
