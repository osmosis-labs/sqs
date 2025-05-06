package atomic

import "sync/atomic"

// Uint64 returns a pointer to an atomic.Uint64 initialized with the given value.
//
// This is a convenience function that simplifies the creation and initialization
// of atomic.Uint64 values. It uses Store to set the value.
//
// Example:
//
//	counter := Uint64(42)
//	fmt.Println(counter.Load()) // Output: 42
func NewUint64(i uint64) *atomic.Uint64 {
	v := &atomic.Uint64{}
	v.Store(i)
	return v
}
