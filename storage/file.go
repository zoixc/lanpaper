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
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if f == nil {
		syscall.Close(fd)
		return nil, errors.New("failed to open media")
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("not a regular media file")
	}
	return f, nil
}
