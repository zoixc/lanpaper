// SPDX-License-Identifier: MIT

package avifcodec

import (
	"context"
	"fmt"
	"sync"
)

// initialization starts trusted runtime compilation at most once. A canceled
// caller stops waiting immediately; it never leaves behind a goroutine per
// request. The one shared compiler has its own bounded lifetime.
type initialization struct {
	once sync.Once
	done chan struct{}
	err  error // published by closing done; never read before it closes
}

func (i *initialization) wait(ctx context.Context, initialize func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.once.Do(func() {
		i.done = make(chan struct{})
		go func() {
			defer close(i.done)
			// Unlike a request goroutine, initialization has no HTTP recovery wrapper.
			// Preserve fail-closed behaviour if the trusted runtime panics on a host.
			defer func() {
				if p := recover(); p != nil {
					i.err = fmt.Errorf("AVIF runtime initialization panic: %v", p)
				}
			}()
			i.err = initialize()
		}()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-i.done:
		return i.err
	}
}
