# Production-readiness review — 2026-10-15

Scope: the current source tree, including static AVIF support. This is a record
of local checks, not a claim that a container or deployment has been certified.

## Corrections

- AVIF runtime initialization is single-flight, bounded separately from image
  execution, and does not keep canceled callers waiting for compilation.
  Initialization errors/panics fail closed; uploads return 503, not a misleading
  malformed-image 400. Cancellation, concurrent waiters and failure persistence
  have regression tests.
- Replacement history writes through the upload service's actual store. A
  regression test replaces original AVIF with WebP while the global store is
  deliberately unrelated. Obsolete global archival and library wrappers,
  duplicate context-free decoder paths and production rate-limit test shims
  were removed.
- The WASM working reservation is derived from its input/output/linear-memory
  limits; decompression of the trusted module has an explicit size limit.
- Repository placeholders no longer produce failed-migration startup warnings;
  real legacy leftovers are still counted and symlinks are excluded.
- Removed the unused dark-logo SVG and its public allowlist entry. Kept fonts
  still used by the base interface, sign-in and design fixtures.
- Corrected systemd's JIT-incompatible memory policy, outdated metrics guidance,
  the mistaken hard-limit description of `GOMEMLIMIT`, and completed roadmap
  entries. Historical remediation notes are labeled as historical.
- Added pinned staticcheck/deadcode gates to CI and release verification, and
  AVIF browser regressions for previews, conversion, original mode and rejected
  animation without replacing live media. Updated localized error messages and
  the service-worker cache version.

## Passed local checks

| Check | Result |
| --- | --- |
| Go 1.27.2 `go test -race -count=1 ./...` | All 12 packages pass |
| Go 1.26.6 `go test -count=1 ./...` | All 12 packages pass |
| `go vet ./...` | Clean |
| staticcheck 2026.2.2 (`v0.8.2`) | Clean |
| deadcode `v0.51.0 -test -filter='^lanpaper' ./...` | No unreachable application functions reported |
| Module integrity | `go mod download` and `go mod verify` pass; regenerated vendor files are byte-identical to the checked tree |
| Linux amd64 and arm64 builds | Both build with `CGO_ENABLED=0` |
| AVIF container fuzzing | 13,018 executions, 10-second target; pass |
| Frontend syntax and contract tests | 91 tests pass |
| CSS formatting, structure and tests | Pass; 9 tests |
| Contrast checks | 204 checks in each of Studio/default modes; no failures |
| Chromium desktop + phone-emulation E2E | 64 pass, 2 intentional project-specific skips |
| AVIF original-mode Chromium E2E | 2 additional tests pass, desktop and phone |
| npm dependency audits | No reported vulnerabilities in either CSS-tool or E2E dependency set |

Browser checks used Chromium 153 via an externally installed test binary;
no browser tooling was added to application dependencies. The two skips are
the desktop-only keyboard-focus check on phone and the phone-only CDP touch
check on desktop. The full suite includes accessibility, sign-in/session
handling, library mutations, history/playlists, import/export, media playback
and visual-layout checks. Visual artifacts are smoke-test captures, not a
human-reviewed pixel-baseline certification.

`govulncheck` 1.8.0 with Go 1.27.2 and official Go vulnerability database HEAD
`d5e0cfac5d260523df858cd4c0f849e248468fb6` (2026-10-11 UTC) reports **zero
reachable vulnerabilities and zero vulnerable imported packages**. It reports
module-only GO-2026-5932 for deprecated `x/crypto/openpgp`, which this application
does not import or call. The maintained crypto dependency remains necessary.

## Deployment checks still required

- **Docker was unavailable here.** Build and smoke-test the actual image,
  including read-only filesystem/non-root execution, health checks and AVIF
  upload under the production seccomp policy. Cross-compilation is not ARM
  runtime testing.
- Run an AVIF upload after applying the real systemd unit. Older units must
  remove/override `MemoryDenyWriteExecute=true`; retain the other hardening.
- WebKit/Safari was not available; phone Chromium emulation is not Safari or a
  physical-device test. Test original AVIF delivery on intended displays.
- Measure warmed peak RSS and throughput under the real memory/CPU limits and
  representative uploads. Neither `GOMEMLIMIT` nor the decoder's reservation
  is a whole-process RSS guarantee.
- Go vulnerability tooling does not scan C codecs embedded in the AVIF WASM
  artifact. Its digest/provenance/licenses are pinned, but upstream codec
  advisories still need review for releases; see [supply-chain guidance](SUPPLY_CHAIN.md).
- External URL tests use controlled transports. No live third-party image-CDN
  request or production-network SSRF/proxy smoke test was performed.

Follow the [deployment checklist](DEPLOYMENT.md#7-pre-launch-checklist) for
secrets, TLS/proxy policy, persistent storage, backups and recovery drills.
