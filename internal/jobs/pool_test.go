// SPDX-License-Identifier: MIT

package jobs

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestPoolBoundsCPUAndMemory(t *testing.T) {
	pool := New(1, 100)
	release, err := pool.TryAcquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.TryAcquire(context.Background()); !errors.Is(err, ErrCapacity) {
		t.Fatalf("second CPU slot: %v", err)
	}
	memoryRelease, err := pool.ReserveMemory(80)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.ReserveMemory(21); !errors.Is(err, ErrCapacity) {
		t.Fatalf("memory overcommit: %v", err)
	}
	if snapshot := pool.Snapshot(); snapshot.Active != 1 || snapshot.MemoryUsed != 80 {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	memoryRelease()
	release()
	if snapshot := pool.Snapshot(); snapshot.Active != 0 || snapshot.MemoryUsed != 0 || snapshot.Completed != 1 {
		t.Fatalf("released snapshot: %+v", snapshot)
	}
}

func TestPoolWaitsAndHonorsCancellation(t *testing.T) {
	pool := New(1, 10)
	entered := make(chan struct{})
	unblock := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(1)
	go func() {
		defer wait.Done()
		_ = pool.Run(context.Background(), 0, func(context.Context) error { close(entered); <-unblock; return nil })
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := pool.Run(ctx, 0, func(context.Context) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
	close(unblock)
	wait.Wait()
}
