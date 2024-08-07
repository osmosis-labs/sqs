package rustffi

/*
#include "../target/release/libsqs_ffi.h"
*/
import "C"
import "github.com/osmosis-labs/osmosis/osmomath"

func NewFFIDivision(startedAt, updatedAt int64, lastestValue, integral osmomath.Dec) (C.struct_FFIDivision, error) {
	latestValueFFIDec, err := NewFFIDecimal(&lastestValue)
	if err != nil {
		return C.struct_FFIDivision{}, err
	}
	integralFFIDec, err := NewFFIDecimal(&integral)
	if err != nil {
		return C.struct_FFIDivision{}, err
	}

	return C.struct_FFIDivision{
		started_at:   C.uint64_t(startedAt),
		updated_at:   C.uint64_t(updatedAt),
		latest_value: latestValueFFIDec,
		integral:     integralFFIDec,
	}, nil
}
