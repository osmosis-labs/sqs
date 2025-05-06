// Package atomic provides concurrency-safe types and utilities.
package atomic

import (
	"sync"
	"sync/atomic"
)

// AtomicCopyOnWrite is a concurrency-safe utility that allows
// atomic reads and synchronized writes. Reads are lock-free
// and fast, while writes are protected by a mutex to ensure
// copy-on-write safety.
type AtomicCopyOnWrite[T any] struct {
	v  atomic.Value
	mu sync.Mutex
}

// NewAtomicCopyOnWrite returns a new instance of AtomicCopyOnWrite.
// Writes are serialized via a mutex, while reads are atomic and lock-free.
func NewAtomicCopyOnWrite[T any]() *AtomicCopyOnWrite[T] {
	return &AtomicCopyOnWrite[T]{
		mu: sync.Mutex{},
	}
}

// Store sets the value atomically. It acquires a lock to ensure
// write consistency, making this safe for concurrent use with Load.
func (c *AtomicCopyOnWrite[T]) Store(v T) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.v.Store(v)
}

// Load returns the most recent value atomically. If no value
// has been stored yet, it returns the zero value of T.
func (c *AtomicCopyOnWrite[T]) Load() T {
	v, ok := c.v.Load().(T)
	if !ok {
		return *new(T)
	}

	return v
}
