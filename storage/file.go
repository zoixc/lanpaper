// SPDX-License-Identifier: MIT

package storage

import (
	"errors"
	"os"
	"syscall"
)

// OpenMedia opens an app-managed file without following a final symlink. The
// path is derived from a validated link name and MIME extension, never from a
// request path. A planted symlink must not make a public link read another
// link's private file (or a file outside the media directory).
func OpenMedia(path string) (*os.File, error) {
	if path == "" {
		return nil, os.ErrNotExist
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("not a regular media file")
	}
	return f, nil
}

// IsReadOnlyError reports whether err means the path lives on a read-only
// filesystem (EROFS), for example a legacy file inside a read_only container
// whose directory is not mounted writable. Such files cannot be removed or
// moved by the server, so callers treat that as a notice, not a failure.
func IsReadOnlyError(err error) bool {
	return errors.Is(err, syscall.EROFS)
}
