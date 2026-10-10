# Retrospective: roadmap PR 29–32

Date: 2026-10-10

Scope: the context-aware store boundary, normalized SQLite backend, resumable
JSON migration and bounded library queries.

## Verification performed

- Ran the shared repository conformance suite against JSON and SQLite.
- Traced transaction rollback, cancellation, record copying and durable commit
  boundaries.
- Reviewed every normalized relation, foreign key action, rename and backup
  round-trip.
- Simulated fresh, interrupted, resumed, source-changed and rolled-back JSON
  migrations.
- Tested pagination/filter combinations, invalid query values, stable sorting,
  legacy truncation and the panel's multi-page loader.
- Re-ran stable, oldstable, frontend, browser E2E and static-image builds.

## Findings and corrections

### SQLite rename paths

The initial SQL rename updated `link_name` and `image_url`, and foreign keys
correctly cascaded, but retained the old `/api/preview/{name}` URL. The returned
record therefore disagreed with the renamed filesystem paths and JSON backend.
The transaction now updates `preview` in the same statement and the backup
round-trip test covers the renamed record.

### Backup directory durability

The migration's backup file was flushed with `fsync`, but its parent directory
was not. A power loss immediately after first creation could therefore lose the
name despite the command claiming a durable backup. The backup-first step now
fsyncs and closes the parent directory before opening SQLite.

### Pagination compatibility test setup

The new 10,000-record compatibility test constructed a zero-value JSON store
and populated it through the startup-only `Set` helper, whose map is nil until
normal loading or creation. This was a test-fixture panic rather than a runtime
path. The fixture now creates the first record durably before filling the
remaining in-memory entries. Production behavior was unaffected.

## Architecture status

The SQLite implementation and migration are intentionally staged: the command
does not silently switch the running server or permit simultaneous JSON/SQLite
writers. The store interface is available at the composition root, but legacy
handlers still require further request-level injection before a production
backend switch. Documentation continues to state this limitation explicitly.

## Outcome

PR 29–32 remain accepted after the rename and durability corrections above.
The background processing work in PR 33 may proceed.
