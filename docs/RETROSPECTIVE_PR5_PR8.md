# Retrospective: roadmap PR 5–8

**Date:** 2026-10-09

## Scope

Reviewed credential verification and rotation, outbound TLS boundaries, durable
persistence, upload publication, migration compatibility, concurrency and the
API/documentation contract introduced by PR 5–8.

## Findings

### Corrected before PR 9: post-rename persistence ambiguity

The initial shared atomic writer returned an ordinary error when directory
open/sync/close failed after rename. Callers interpreted every error as a
pre-commit failure and retained old in-memory state, although disk already held
the new state. A restart could therefore reveal an operation previously
reported as failed.

The writer now distinguishes `CommittedError`: rename is the persistence commit
point. Session and wallpaper stores log failed durability confirmation but
publish the matching in-memory state and report logical success. Fault tests
assert which failures occur before versus after commit.

## Security and integration review

- Argon2id parameters are parsed with allocation bounds; malformed hashes fail
  closed. Hash credentials take precedence over deprecated plaintext and both
  browser and machine authentication use the same verifier.
- Credential fingerprints contain no password material and invalidate sessions
  after rotation. Pre-fingerprint files receive one compatibility migration.
- Remote-target and HTTPS-proxy certificate opt-outs are independent. The
  deprecated broad switch is explicit and documented.
- Session and wallpaper writes share the same durable boundary semantics.
- Upload metadata is committed only after all required files publish; failures
  roll published files back in reverse order. Final cleanup is best effort only
  after metadata commit.
- No API response shape or frontend contract changed in PR 5–8.

## Test and documentation review

Stable/oldstable Go tests, formatting, vet, race-enabled unit tests, build,
vulnerability scan, frontend, browser E2E and Docker smoke checks are required
by CI. New fault tests avoid permission-dependent behavior. README, SECURITY,
deployment guidance, changelog and roadmap describe migration and compatibility
impact.

## Residual risks

- A directory-sync failure after rename means the logical operation completed
  but power-loss durability is uncertain; this is now logged without creating
  memory/disk disagreement.
- Upload process-crash recovery still requires the offline audit/repair tooling
  planned in PR 9–10; in-process errors and disconnects are bounded and cleaned
  up by the transaction path.
