// SPDX-License-Identifier: MIT

// Package atomicfile durably replaces small state files and exposes every I/O
// boundary through FS so tests can inject failures without permission tricks.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

type File interface {
	Name() string
	Write([]byte) (int, error)
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type Dir interface {
	Sync() error
	Close() error
}

type FS interface {
	CreateTemp(dir, pattern string) (File, error)
	Rename(oldPath, newPath string) error
	Remove(path string) error
	OpenDir(path string) (Dir, error)
}

type OSFS struct{}

func (OSFS) CreateTemp(dir, pattern string) (File, error) { return os.CreateTemp(dir, pattern) }
func (OSFS) Rename(oldPath, newPath string) error         { return os.Rename(oldPath, newPath) }
func (OSFS) Remove(path string) error                     { return os.Remove(path) }
func (OSFS) OpenDir(path string) (Dir, error)             { return os.Open(path) }

func Write(fs FS, path, pattern string, body []byte, mode ...fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := fs.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	name := tmp.Name()
	if len(mode) > 0 {
		if err := tmp.Chmod(mode[0]); err != nil {
			_ = tmp.Close()
			_ = fs.Remove(name)
			return fmt.Errorf("chmod temp: %w", err)
		}
	}
	cleanup := func() { _ = tmp.Close(); _ = fs.Remove(name) }
	if _, err := tmp.Write(body); err != nil {
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = fs.Remove(name)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := fs.Rename(name, path); err != nil {
		_ = fs.Remove(name)
		return fmt.Errorf("rename temp: %w", err)
	}
	d, err := fs.OpenDir(dir)
	if err != nil {
		return fmt.Errorf("open directory after rename: %w", err)
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return fmt.Errorf("sync directory after rename: %w", err)
	}
	if err := d.Close(); err != nil {
		return fmt.Errorf("close directory after rename: %w", err)
	}
	return nil
}
