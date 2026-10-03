// SPDX-License-Identifier: MIT

package storage

import (
	"slices"
	"sort"
	"sync"
)

// LinkLocks coordinates file operations with metadata mutations on the same
// link. Unlike an unbounded sync.Map of mutexes, unused locks are removed.
var linkLocks = struct {
	sync.Mutex
	byName map[string]*linkLock
}{byName: make(map[string]*linkLock)}

type linkLock struct {
	sync.Mutex
	name string
	refs int // includes goroutines waiting to acquire this lock
}

// LockLinks locks one or more link names in stable order. Always call the
// returned function, usually with defer. Never acquire these while holding a
// Store lock (the order is link locks -> store lock).
func LockLinks(names ...string) func() {
	keys := append([]string(nil), names...)
	sort.Strings(keys)
	locks := make([]*linkLock, 0, len(keys))
	linkLocks.Lock()
	for i, key := range keys {
		if i > 0 && key == keys[i-1] {
			continue
		}
		l := linkLocks.byName[key]
		if l == nil {
			l = &linkLock{name: key}
			linkLocks.byName[key] = l
		}
		l.refs++
		locks = append(locks, l)
	}
	linkLocks.Unlock()
	for _, l := range locks {
		l.Lock()
	}
	return func() {
		// Release in reverse order. Never remove a lock if someone is waiting.
		linkLocks.Lock()
		for _, l := range slices.Backward(locks) {
			l.Unlock()
			l.refs--
			if l.refs == 0 {
				delete(linkLocks.byName, l.name)
			}
		}
		linkLocks.Unlock()
	}
}
