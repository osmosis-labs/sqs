package atomic

import (
	"encoding/json/v2"
	"strconv"
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

// MarshalJSON implements json.Marshaler. The value is a JSON number.
func (a *Uint64) MarshalJSON() ([]byte, error) {
	return strconv.AppendUint(nil, a.Load(), 10), nil
}

// UnmarshalJSON implements json.Unmarshaler. It accepts a JSON number that fits a uint64;
// null stores zero.
func (a *Uint64) UnmarshalJSON(data []byte) error {
	var val uint64
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}

	a.Store(val)

	return nil
}
