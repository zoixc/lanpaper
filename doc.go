// SPDX-License-Identifier: MIT

// Lanpaper serves one mutable media file per stable URL: a link name that
// never changes while the bytes behind it are replaced, versioned, rotated or
// rolled back.
//
// Command lanpaper is a single self-contained binary. Package main owns the
// process lifecycle and nothing else:
//
//   - configuration (env > config.json > defaults) and its startup validation,
//   - the data directories, the metadata store and the one-shot media
//     migration from the legacy static/images location,
//   - the HTTP handler chain (gzip → panic recovery → routing) and the
//     listener, with or without TLS,
//   - graceful shutdown on SIGINT/SIGTERM, waiting for in-flight uploads.
//
// Routes are declared in newMux; the same chain is used by the end-to-end
// tests in main_test.go, so the tests exercise the real compression,
// authentication, CSRF and routing behaviour rather than bare handlers.
package main
