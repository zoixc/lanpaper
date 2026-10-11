// SPDX-License-Identifier: MIT

package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

// LockFileName is created inside the data directory while a Lanpaper process
// or a repair operation owns it.
const LockFileName = ".lanpaper.lock"

// LockDataDir takes an exclusive, non-blocking lock on the data directory. Two
// processes writing the same SQLite database and session table would race, so
// a second start fails with a clear error instead. The returned function
// releases the lock. The OS drops the lock if the process crashes.
func LockDataDir(dir string) (func(), error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, LockFileName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if lockFile(f) != nil {
		f.Close()
		return nil, fmt.Errorf("data directory %s is already in use by another Lanpaper process or repair; stop it before starting", dir)
	}
	return func() {
		unlockFile(f)
		f.Close()
	}, nil
}
