// SPDX-License-Identifier: MIT

// Package middleware holds everything that wraps a handler rather than
// implementing one: security headers, CSRF defence, authentication, publish
// keys, rate limiting, gzip and panic recovery.
//
// Ordering matters and is fixed in newMux: security headers and the CSRF check
// run before authentication, which runs before the rate limit, which runs
// before the handler. Gzip is the outermost layer and Recover sits just inside
// it, so a recovered 500 still travels through the normal writer chain.
//
// Two invariants are worth keeping in mind when adding a wrapper:
//
//   - Response writers must stay transparent. Anything that wraps one has to
//     delegate ReadFrom and Unwrap, or media loses sendfile and uploads lose
//     their per-request deadlines.
//   - Nothing on the request path may read global state that another goroutine
//     writes. Settings are published through atomic snapshots in config, and
//     counters are guarded by a single mutex.
//
// Authorisation for public links (public/local/token/auth) lives in access.go
// and shares its brute-force budget with the admin login, so a token cannot be
// guessed faster than a password.
package middleware
