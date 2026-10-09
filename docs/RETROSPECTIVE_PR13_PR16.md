# Retrospective: roadmap PR 13–16

**Date:** 2026-10-09

## Scope

Reviewed browser workflow validity, accessibility/cross-engine behavior, fuzz
target compilation and coverage, token-bucket semantics, CI diagnostics and
resource/privacy boundaries introduced by PR 13–16.

## Findings corrected before PR 17

### Browser export/import coverage bypassed the real UI module

The first lifecycle test approximated export/import with direct API calls and
only checked an obsolete token value. It could pass while the downloadable file
leaked the current bearer token. The test now downloads through the settings
export action, checks both old and current tokens are absent, deletes the link,
imports the actual file through `LanpaperBackup.importData`, confirms the UI
prompt and verifies recreation.

### Multipart fuzz cleanup used the wrong method name

The new fuzz corpus did not compile because it called `cleanup` instead of the
form's `close` method. This was corrected before proceeding. Session persistence
and media-signature fuzz targets were also added to close gaps found while
matching implementation to the roadmap wording.

### Accessibility checks found an unnamed file input

Axe identified the visually hidden media picker as a critical unlabeled form
control and WebKit measured secondary text just below the required 4.5:1
contrast. The input now has an accessible name and the light secondary color is
darker across engines. Phone runs also exposed desktop-keyboard
and actionability assumptions: physical-keyboard focus restoration is asserted
on both desktop engines, while the narrow 200% zoom check activates the visible
touch control without requiring its unzoomed bounding box to remain inside the
initial viewport.

## Token-bucket review

- Capacity is sustained rate plus configured burst; refill uses only the
  sustained rate, preventing burst allowance from increasing long-term rate.
- Public download, upload/regeneration, login-failure and publish-key-failure
  state is namespaced independently.
- IPv6 /64 grouping, trusted-proxy parsing, sharding and periodic bounded-idle
  cleanup remain intact.
- `Retry-After` is derived from time to the next token rather than a window
  reset. Disabled-limit warnings make unsafe single-instance configuration
  explicit.
- Existing upload/decode semaphores remain the global heavy-work ceiling.

## CI and residual risks

Chromium and WebKit browser suites pass after the fixes. Docker Hub repeatedly
blocked otherwise unchanged smoke builds at its unauthenticated pull-rate
limit, so Dockerfile base images are build arguments and pull-request CI uses
the public ECR mirror; local/release defaults remain unchanged. Browser E2E is
serial because it intentionally shares one process-local rate limiter and test
server. Fuzz seeds run in ordinary CI; longer randomized fuzz campaigns remain
appropriate for scheduled or release hardening runs.
