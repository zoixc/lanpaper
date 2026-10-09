// SPDX-License-Identifier: MIT

package atomicfile

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

type faultFS struct {
	at               string
	removed, renamed bool
}
type faultFile struct {
	owner  *faultFS
	closed bool
}
type faultDir struct{ owner *faultFS }

func (f *faultFS) fail(at string) error {
	if f.at == at {
		return errors.New("injected " + at)
	}
	return nil
}
func (f *faultFS) CreateTemp(string, string) (File, error) {
	if e := f.fail("create"); e != nil {
		return nil, e
	}
	return &faultFile{owner: f}, nil
}
func (f *faultFS) Rename(string, string) error {
	if e := f.fail("rename"); e != nil {
		return e
	}
	f.renamed = true
	return nil
}
func (f *faultFS) Remove(string) error { f.removed = true; return f.fail("remove") }
func (f *faultFS) OpenDir(string) (Dir, error) {
	if e := f.fail("open-dir"); e != nil {
		return nil, e
	}
	return &faultDir{f}, nil
}
func (f *faultFile) Name() string            { return "/state/.tmp" }
func (f *faultFile) Chmod(fs.FileMode) error { return f.owner.fail("chmod") }
func (f *faultFile) Write(p []byte) (int, error) {
	if e := f.owner.fail("write"); e != nil {
		return 0, e
	}
	return len(p), nil
}
func (f *faultFile) Sync() error  { return f.owner.fail("sync") }
func (f *faultFile) Close() error { f.closed = true; return f.owner.fail("close") }
func (d *faultDir) Sync() error   { return d.owner.fail("sync-dir") }
func (d *faultDir) Close() error  { return d.owner.fail("close-dir") }

func TestWriteFaultMatrix(t *testing.T) {
	for _, at := range []string{"create", "chmod", "write", "sync", "close", "rename", "open-dir", "sync-dir", "close-dir"} {
		t.Run(at, func(t *testing.T) {
			fake := &faultFS{at: at}
			err := Write(fake, "/state/data.json", ".tmp-*", []byte("state"), 0o600)
			if err == nil || !strings.Contains(err.Error(), at) {
				t.Fatalf("error=%v, want injected %s", err, at)
			}
			if (at == "create" || at == "chmod" || at == "write" || at == "sync" || at == "close" || at == "rename") && fake.renamed {
				t.Fatal("published after pre-rename failure")
			}
			if at != "create" && at != "open-dir" && at != "sync-dir" && at != "close-dir" && !fake.removed {
				t.Fatal("temporary file was not cleaned up")
			}
		})
	}
}

func TestWriteSuccess(t *testing.T) {
	fake := &faultFS{}
	if err := Write(fake, "/state/data.json", ".tmp-*", []byte("state")); err != nil {
		t.Fatal(err)
	}
	if !fake.renamed {
		t.Fatal("state was not published")
	}
}
