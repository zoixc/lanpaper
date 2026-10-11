//go:build !unix

// SPDX-License-Identifier: MIT

package storage

import "os"

// Without flock the lock file is only a marker. Lanpaper's supported runtime is
// Linux containers, where lock_unix.go is used.
func lockFile(*os.File) error { return nil }
func unlockFile(*os.File)     {}
