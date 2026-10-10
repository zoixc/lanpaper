# Retrospective: roadmap PR 25–28

Date: 2026-10-10

Scope: accessible overlays, validated bulk import, offline/retry UX and measured
JSON-storage limits.

## Verification performed

- Traced nested overlay open/close, dismissal, focus restoration, inertness,
  scroll lock, announcements and visual-viewport handling.
- Followed bulk import from file parsing through whole-import dry runs, bounded
  durable commits, cancellation and final/partial reports.
- Reviewed online/offline messaging, retry callbacks, upload operation keys and
  service-worker fetch policy.
- Re-ran 1k/10k/50k storage benchmarks on a hosted runner and compared
  serialization-only mutations with end-to-end fsynced atomic writes.
- Re-ran frontend contracts and the stable, oldstable, E2E and Docker matrix.

## Findings

### Overlay controller

Nested confirmation dialogs correctly make the parent inert and restore it on
close. Escape and scrim dismissal resolve pending confirmations once, and focus
returns to a connected origin. The visual-viewport listener only scrolls a
focused control inside the active overlay. No regression was found.

### Bulk import

Validation happens across all client batches before mutation starts. Each
server batch validates completely before a single atomic store commit, existing
links remain untouched, and token links receive newly generated secrets. The
PR 21–24 retrospective already corrected the missing partial report on
cancellation. No further defect was found.

### Offline/retry and duplicate submission

Two issues were found and fixed before PR 29:

1. The offline banner said changes were “paused”, but the UI intentionally keeps
   controls available and lets network requests fail/retry. All translations
   now state truthfully that network operations may fail until connectivity
   returns.
2. File and URL uploads used different operation keys. A user could therefore
   start both against the same link and race two mutations. Both paths now use
   the same `upload:<link>` key; the contract test starts a file upload and
   verifies that a concurrent URL upload is rejected.

The service worker remains restricted to immutable public application assets;
admin, API and mutable media responses are never served from cache.

### Storage limits

Measured results support the published policy. At 10k records, a durable atomic
rewrite was about 27 ms and list construction about 1.8 ms on the runner. At
50k, durable rewrite rose to about 132 ms with roughly 48 MB allocated, while a
list allocated about 15 MB. The 10k supported ceiling and 50k JSON hard boundary
are conservative and include operational triggers for slower disks and richer
records.

## Outcome

PR 25–28 remain accepted after the two offline/upload corrections above. The
next storage abstraction work may proceed.
