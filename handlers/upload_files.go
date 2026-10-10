// SPDX-License-Identifier: MIT

// Staging and publishing of uploaded files: every write lands in a temporary
// sibling first, so a failed request can never truncate or replace a live file.
package handlers

import (
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"

	"lanpaper/storage"
)

// copyFile writes to a temporary sibling and renames it into place. io.Copy
// can use optimized file-to-file copies; a bounded reader still protects
// against a local source growing after its initial size check.
func copyFile(dst string, src io.Reader, limit int64) error {
	return writeFileAtomic(dst, func(out *os.File) error {
		n, err := io.Copy(out, io.LimitReader(src, limit+1))
		if err == nil && n > limit {
			err = errMediaTooLarge
		}
		return err
	})
}

// writeFileAtomic writes a temporary sibling of dst, flushes it to stable
// storage and renames it into place. A failed or interrupted write can never
// truncate an existing file at dst.
func writeFileAtomic(dst string, write func(*os.File) error) error {
	out, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*"+filepath.Ext(dst))
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	writeErr := write(out)
	if writeErr == nil {
		writeErr = out.Sync()
	}
	closeErr := out.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp, dst)
}

func stagePath(dir, ext string) (string, error) {
	f, err := os.CreateTemp(dir, ".upload-*."+ext)
	if err != nil {
		return "", err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// publishStaged atomically replaces a destination while retaining a hardlink
// to its previous contents. On a failed metadata commit, Rollback restores
// the previous file even if the extension/path was unchanged. Hardlinks are
// constant-time; the copy fallback supports filesystems without hardlinks.
type publishedFile struct {
	dst, backup    string
	rollbackAction func()
	finishAction   func()
}

func publishStaged(stage, dst string, maxBytes int64) (publishedFile, error) {
	p := publishedFile{dst: dst}
	fi, err := os.Lstat(dst)
	if err == nil {
		if !fi.Mode().IsRegular() {
			return p, errors.New("existing media is not a regular file")
		}
		p.backup, err = stagePath(filepath.Dir(dst), filepath.Ext(dst)[1:])
		if err != nil {
			return p, err
		}
		os.Remove(p.backup)
		if err = os.Link(dst, p.backup); err != nil {
			f, openErr := storage.OpenMedia(dst)
			if openErr != nil {
				return p, openErr
			}
			err = copyFile(p.backup, f, maxBytes)
			f.Close()
			if err != nil {
				os.Remove(p.backup)
				return p, err
			}
		}
	} else if !os.IsNotExist(err) {
		return p, err
	}
	if err := os.Rename(stage, dst); err != nil {
		if p.backup != "" {
			os.Remove(p.backup)
		}
		return p, err
	}
	return p, nil
}

func (p publishedFile) rollback() {
	if p.rollbackAction != nil {
		p.rollbackAction()
		return
	}
	if p.backup != "" {
		if err := os.Rename(p.backup, p.dst); err != nil {
			log.Printf("Critical: could not restore media %s: %v", p.dst, err)
		}
	} else if err := os.Remove(p.dst); err != nil && !os.IsNotExist(err) {
		log.Printf("Error removing failed upload %s: %v", p.dst, err)
	}
}

func (p publishedFile) finish() {
	if p.finishAction != nil {
		p.finishAction()
		return
	}
	if p.backup != "" {
		if err := os.Remove(p.backup); err != nil && !os.IsNotExist(err) {
			log.Printf("Error removing media backup %s: %v", p.backup, err)
		}
	}
}
