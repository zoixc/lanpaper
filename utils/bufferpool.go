package utils

import (
	"sync"
)

// BufferPool provides a pool of reusable byte slices to reduce allocations
// during file copy operations. Buffers are 1MB by default.
type BufferPool struct {
	pool *sync.Pool
	size int
}

// NewBufferPool creates a buffer pool with the specified buffer size.
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		size: size,
		pool: &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, size)
				return &buf
			},
		},
	}
}

// Get retrieves a buffer from the pool.
func (p *BufferPool) Get() []byte {
	return *p.pool.Get().(*[]byte)
}

// Put returns a buffer to the pool for reuse.
func (p *BufferPool) Put(buf []byte) {
	if cap(buf) != p.size {
		// Don't put back buffers with wrong capacity
		return
	}
	// Clear buffer before returning to pool (security)
	for i := range buf {
		buf[i] = 0
	}
	p.pool.Put(&buf)
}
