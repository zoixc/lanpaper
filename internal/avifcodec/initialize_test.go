// SPDX-License-Identifier: MIT

package avifcodec

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestInitializationCancellationDoesNotPoisonRuntime(t *testing.T) {
	var init initialization
	started, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	compile := func() error { calls.Add(1); close(started); <-finish; return nil }
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- init.wait(ctx, compile) }()
	<-started
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		close(finish)
		t.Fatal("canceled request remained blocked compiling AVIF")
	}
	close(finish)
	if err := init.wait(t.Context(), compile); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatal("runtime initialized more than once")
	}
}

func TestConcurrentInitializationAndFailure(t *testing.T) {
	var init initialization
	sentinel := errors.New("executable memory not permitted")
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			err := init.wait(t.Context(), func() error { calls.Add(1); return sentinel })
			if !errors.Is(err, sentinel) {
				t.Errorf("initialization error: %v", err)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatal("runtime initialized more than once")
	}
}

func TestInitializationPanicIsFailClosed(t *testing.T) {
	var init initialization
	if err := init.wait(t.Context(), func() error { panic("runtime failure") }); err == nil || !strings.Contains(err.Error(), "runtime failure") {
		t.Fatal(err)
	}
}
