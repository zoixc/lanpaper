# Retrospective: roadmap PR 21–24

Date: 2026-10-10

Scope: injected remote fetching, the frontend API/error module, extracted state
selectors, and frontend feature boundaries. The review also followed these
changes through PR 25–26 because those PRs are their first production callers.

## Verification performed

- Reviewed the complete diff from `cd60d82` through `2010dcb`, plus the overlay
  and bulk-import integrations built on those abstractions.
- Rechecked SSRF resolution/pinning, redirects, target/proxy TLS separation,
  cancellation and the independent network/decode semaphores.
- Traced native-module URLs and startup ordering after the earlier duplicate
  execution regression.
- Checked state normalization/selectors for mutation leaks and fallback paths.
- Traced every capability exposed to export/import and confirmed that the old
  broad `window.LanpaperApp` object is absent.
- Ran frontend syntax, unit and contract tests and relied on the completed
  stable, oldstable, browser E2E and Docker matrices for each final commit.

## Findings

### Remote fetch ownership and boundaries

`RemoteFetcher` preserves the original security model: each hop is resolved and
validated, the accepted IP is pinned into the dial, redirects are checked again,
and target TLS controls remain separate from proxy TLS controls. Network waits
and media decoding occupy different bounded semaphores, so a slow remote server
does not consume a decode slot. The compatibility wrapper still uses the
process default intentionally; request work through `UploadService` owns the
injected dependency.

No regression was found in this area.

### Native module identity and startup

The initial PR 22 implementation exposed an important browser-module identity
hazard: the same module imported once through an unversioned relative URL and
once through a versioned script URL executes twice. This was caught by E2E and
fixed before PR 22 was considered complete. The final page uses ordered module
scripts without the duplicate import, and current E2E covers the resulting
single initialization.

### State and feature isolation

State defaults, normalization and selectors are now pure and directly tested.
The feature registry freezes the capability objects, while snapshots clone each
link before export code receives it. Upload orchestration depends on injected
request/update/UI capabilities rather than mutable global state. The remaining
`window.LanpaperBackup` object is a deliberately narrow three-action automation
compatibility surface; it does not expose panel state or request internals.

No new mutation or initialization regression was found.

## Defect found after the four-PR boundary

Bulk import, which is the first long-running consumer of the new facade, kept a
per-record report during each batch but discarded that report when the user
cancelled. Records committed by earlier atomic batches therefore had no report,
contradicting the progress/cancellation contract.

Fixed before PR 27: cancellation errors now carry the partial report, including
`cancelled`, `processed`, created/skipped counts and completed per-record
results. The UI downloads that report and refreshes the library before showing
the cancellation notification.

## Documentation corrections

Roadmap entries 21–26 were still labelled “full CI pending” after their final
stable, oldstable, frontend, E2E and Docker runs had passed. This retrospective
updates those statuses and records the delayed PR 21–24 review explicitly.

## Outcome

PR 21–24 remain accepted. The only newly identified behavioural defect was the
missing cancellation report in their later bulk-import consumer, and it was
fixed before beginning the next roadmap PR.
