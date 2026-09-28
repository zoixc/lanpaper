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
