// SPDX-License-Identifier: MIT

// Package handlers implements the HTTP API and the public media endpoint.
//
// The rules every handler follows:
//
//   - Validate before doing work. Link names, categories, access levels,
//     extensions and numeric selectors are checked against the allow-lists in
//     config and utils; anything else is a 400 or 404, never a path.
//   - Authorise before resolving. A per-link access decision is made before a
//     version, playlist item or file is opened, so no selector can widen what a
//     link exposes.
//   - Fail loudly, answer generically. Internal errors are logged with context
//     and returned as a short message; nothing about the filesystem, the
//     configuration or a stack is ever sent to a client.
//   - Publish atomically. Media is staged in its destination directory and
//     renamed into place, keeping a hardlink to the previous bytes so a failed
//     metadata commit can be rolled back and a successful one can be archived.
//
// Public serves the mutable URL itself (/{name}, ?v=, ?i=, extension and
// /latest aliases) through http.ServeContent, which keeps ETag, Last-Modified,
// range requests and sendfile. The admin API (/api/…) is paginated and answers
// with the same link object the panel renders.
package handlers
