// SPDX-License-Identifier: MIT

// Package storage owns the metadata store and the on-disk media layout.
//
// The store is copy-on-write: readers take an RLock and get a shallow clone of
// a record or of the sorted snapshot, writers clone the map, apply the edit and
// commit it. A commit writes only the records that changed, in one SQLite
// transaction on data/wallpapers.db (synchronous=FULL in WAL mode). A crash
// therefore leaves either the old state or the new one, never a partial record.
// Because snapshots are shared, a caller must never mutate a slice it got from
// Get or GetAll — build a new one inside an Update closure.
//
// Layout (all paths derived from a validated link name, never from a request):
//
//	data/media/{link}.{ext}          the file a URL serves
//	data/previews/{link}.webp        admin-panel thumbnail
//	data/history/{link}/{version}.{ext}  replaced versions, newest kept first
//	data/items/{link}/{id}.{ext}     extra playlist files behind the same URL
//
// LinkLocks serialises file work and metadata edits for one link; the lock map
// is reference counted, so it does not grow with the number of links ever seen.
// Never acquire a link lock while holding the store lock — the order is always
// link locks, then store lock.
//
// Files are opened with OpenMedia, which refuses symlinks and anything that is
// not a regular file, and history.go, playlist.go and stats.go add the three
// optional features (version history, playlists with stateless rotation, and
// in-memory access counters) without changing that layout.
package storage
