package atomic

import (
	"encoding/json"
	"sync/atomic"
)

// Uint64 returns a pointer to an atomic.Uint64 initialized with the given value.
//
// This is a convenience function that simplifies the creation and initialization
// of atomic.Uint64 values. It uses Store to set the value.
//
// Example:
//
//	counter := Uint64(42)
//	fmt.Println(counter.Load()) // Output: 42
func NewUint64(i uint64) *Uint64 {
	v := &Uint64{}
	v.Store(i)
	return v
}

// Uint64 is a wrapper around atomic.Uint64
// that provides JSON marshaling and unmarshaling capabilities.
type Uint64 struct {
	atomic.Uint64
}

// MarshalJSON implements json.Marshaler
func (a *Uint64) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.Load())
}

// UnmarshalJSON implements json.Unmarshaler
func (a *Uint64) UnmarshalJSON(data []byte) error {
	var val uint64
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}

	a.Store(val)

	return nil
}
