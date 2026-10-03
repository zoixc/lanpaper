// SPDX-License-Identifier: MIT

package utils

import (
	"fmt"
	"os"
)

// OpenExternalFile opens a gallery file relative to a filesystem root.
// os.Root resolves every symlink while keeping the opened descriptor inside
// baseDir, even if a symlink is replaced between validation and open. Never
// resolve a name with EvalSymlinks and then reopen the resulting absolute path.
func OpenExternalFile(baseDir, name string) (*os.File, error) {
	if name == "" || !IsValidLocalPath(name) {
		return nil, fmt.Errorf("invalid relative path")
	}
	root, err := os.OpenRoot(baseDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, fmt.Errorf("not a regular file")
	}
	return f, nil
}
