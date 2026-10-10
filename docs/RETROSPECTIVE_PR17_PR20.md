# Retrospective: roadmap PR 17–20

**Date:** 2026-10-10

## Scope

Reviewed composition ownership, mutable runtime isolation, compatibility
adapters, upload transaction/service boundaries, history counters, playlist
file ordering, error contracts and concurrent test behavior introduced by PR
17–20.

## Finding corrected before PR 21

### Compatibility services captured a replaceable global store

The first service extraction created package-level service objects holding the
value of `storage.Global` at package initialization. Legacy integration tests
replace that pointer with an isolated store; direct `Upload`, history and
playlist handlers continued using the old object. This caused cross-test state,
races and incorrect status codes. Compatibility entry points now resolve
`storage.Global` at call time. App-owned production services remain stable,
injected references.

## Boundary review

- `App` is the only production route composition root and owns service,
  session, limiter and logger dependencies.
- Session and limiter constructors produce independent mutable stores. Package
  defaults remain compatibility surfaces for direct middleware tests; they are
  not shared by the App dependency graph.
- Upload publication still follows stage, publish, metadata commit and reverse
  rollback ordering. `UploadError` adds machine-testable stage context without
  changing the generic HTTP error body.
- History rollback changes files before metadata and restores them if commit
  fails. Store-scoped history deletion updates the byte counter before deleting
  archived bytes.
- Playlist removal commits metadata first and only then removes bytes. A failed
  commit therefore cannot leave a listed item missing.
- Typed service errors preserve `errors.Is` behavior for not-found and storage
  failures, retaining existing HTTP status mappings.

## Residual work

Some package-level compatibility handlers still exist for tests and external Go
callers. Subsequent service migrations should remove them only with equivalent
contract tests; production App routes should continue using injected services
directly. History byte totals remain process-wide because all stores currently
share one on-disk data root; multi-root storage is outside the supported model.
