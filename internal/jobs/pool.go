// SPDX-License-Identifier: MIT

package jobs

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var ErrCapacity = errors.New("processing capacity reached")

type Pool struct {
	cpu    chan struct{}
	memory struct {
		sync.Mutex
		used  int64
		limit int64
	}
	queued atomic.Int64
	active atomic.Int64
	done   atomic.Int64
}

type Snapshot struct {
	Queued      int64 `json:"queued"`
	Active      int64 `json:"active"`
	Completed   int64 `json:"completed"`
	MemoryUsed  int64 `json:"memoryUsed"`
	MemoryLimit int64 `json:"memoryLimit"`
}

func New(cpuSlots int, memoryLimit int64) *Pool {
	if cpuSlots < 1 {
		cpuSlots = 1
	}
	pool := &Pool{cpu: make(chan struct{}, cpuSlots)}
	pool.memory.limit = memoryLimit
	return pool
}

func (p *Pool) acquireCPU(ctx context.Context, wait bool) (func(), error) {
	p.queued.Add(1)
	if wait {
		select {
		case p.cpu <- struct{}{}:
		case <-ctx.Done():
			p.queued.Add(-1)
			return nil, ctx.Err()
		}
	} else {
		select {
		case p.cpu <- struct{}{}:
		default:
			p.queued.Add(-1)
			return nil, ErrCapacity
		}
	}
	p.queued.Add(-1)
	p.active.Add(1)
	return func() { <-p.cpu; p.active.Add(-1); p.done.Add(1) }, nil
}

func (p *Pool) Run(ctx context.Context, memory int64, work func(context.Context) error) error {
	releaseCPU, err := p.acquireCPU(ctx, true)
	if err != nil {
		return err
	}
	defer releaseCPU()
	releaseMemory, err := p.ReserveMemory(memory)
	if err != nil {
		return err
	}
	defer releaseMemory()
	if err := ctx.Err(); err != nil {
		return err
	}
	return work(ctx)
}

func (p *Pool) TryAcquire(ctx context.Context) (func(), error) { return p.acquireCPU(ctx, false) }

func (p *Pool) ReserveMemory(amount int64) (func(), error) {
	if amount <= 0 {
		return func() {}, nil
	}
	p.memory.Lock()
	if amount > p.memory.limit || p.memory.used+amount > p.memory.limit {
		p.memory.Unlock()
		return nil, ErrCapacity
	}
	p.memory.used += amount
	p.memory.Unlock()
	return func() { p.memory.Lock(); p.memory.used -= amount; p.memory.Unlock() }, nil
}

func (p *Pool) Snapshot() Snapshot {
	p.memory.Lock()
	used, limit := p.memory.used, p.memory.limit
	p.memory.Unlock()
	return Snapshot{Queued: p.queued.Load(), Active: p.active.Load(), Completed: p.done.Load(), MemoryUsed: used, MemoryLimit: limit}
}
