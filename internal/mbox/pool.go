package mbox

import "bytes"

// BufferPool is a bounded pool of reusable byte buffers.
// Get blocks when all buffers are in use, providing natural back-pressure.
type BufferPool struct {
	ch        chan *bytes.Buffer
	initCap   int
	shrinkCap int
}

// NewBufferPool creates a pool of size pre-allocated buffers.
// Each buffer starts with initCap capacity. Buffers that grow beyond
// shrinkCap are replaced with fresh ones on Put to prevent one outlier
// message from permanently inflating memory.
func NewBufferPool(size, initCap, shrinkCap int) *BufferPool {
	p := &BufferPool{
		ch:        make(chan *bytes.Buffer, size),
		initCap:   initCap,
		shrinkCap: shrinkCap,
	}
	for range size {
		p.ch <- bytes.NewBuffer(make([]byte, 0, initCap))
	}
	return p
}

// Get returns a ready-to-use buffer from the pool.
// Blocks if all buffers are currently in use.
func (p *BufferPool) Get() *bytes.Buffer {
	buf := <-p.ch
	buf.Reset()
	return buf
}

// Put returns a buffer to the pool. If the buffer's capacity exceeds
// shrinkCap, it is discarded and replaced with a fresh buffer.
func (p *BufferPool) Put(buf *bytes.Buffer) {
	if buf.Cap() > p.shrinkCap {
		buf = bytes.NewBuffer(make([]byte, 0, p.initCap))
	}
	p.ch <- buf
}
