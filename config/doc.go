// SPDX-License-Identifier: MIT

// Package config holds every runtime setting and the limits that keep a
// request from turning into unbounded work.
//
// Load applies a fixed precedence — environment variables over config.json
// over built-in defaults — and validate clamps each value into its documented
// range, so a typo degrades one setting instead of the whole service. Invalid
// input is logged and ignored; nothing is fatal except the startup checks in
// main.
//
// Two rules keep the request path cheap:
//
//   - Current is written once during Load and only read afterwards. Anything
//     that needs parsing (trusted proxy, CORS origins, publish-key digests) is
//     pre-parsed by RefreshDerived into an atomic pointer, so a request never
//     splits a string, hashes a key or takes a lock.
//   - Secrets are never persisted. PublishKeys carries json:"-" and is only
//     read from the environment; logged output is limited to a short
//     fingerprint.
//
// Constants in constants.go are the single source of truth for the bounds that
// handlers and storage enforce (upload size, decode budget, history and
// playlist limits, timeouts, allowed extensions and access levels).
package config
