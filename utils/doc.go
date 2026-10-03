// SPDX-License-Identifier: MIT

// Package utils holds the validation primitives the handlers and the storage
// layer share: link names, filesystem paths, media magic bytes and the
// resolution rules that keep an outbound download away from private addresses.
//
// Everything here is a pure predicate or a guarded open — no state, no I/O
// beyond the file descriptor it returns. IsValidLinkName and IsValidLocalPath
// are allow-lists, ValidateFileType compares magic bytes instead of trusting an
// extension or a Content-Type, and ResolvePublicURL pins a vetted IP so a
// redirect or a second DNS answer cannot turn a download into an SSRF.
package utils
