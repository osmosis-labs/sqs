package rustffi

/*
#include "../target/release/libsqs_ffi.h"
*/
import "C"
import (
	"errors"
	"unsafe"

	"github.com/osmosis-labs/osmosis/osmomath"
)

func NewFFIDivision(startedAt, updatedAt uint64, lastestValue, prevValue osmomath.Dec) (C.struct_FFIDivision, error) {
	elapsedTime := updatedAt - startedAt
	integral := prevValue.MulInt(osmomath.NewIntFromUint64(elapsedTime))
	return NewFFIDivisionRaw(startedAt, updatedAt, lastestValue, integral)
}

func NewFFIDivisionRaw(startedAt, updatedAt uint64, lastestValue, integral osmomath.Dec) (C.struct_FFIDivision, error) {
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

func NewFFIDivisions(divisions []struct {
	StartedAt   uint64
	UpdatedAt   uint64
	LatestValue osmomath.Dec
	PrevValue   osmomath.Dec
}) ([]C.struct_FFIDivision, error) {
	ffidivisions := make([]C.struct_FFIDivision, len(divisions))
	for i, division := range divisions {
		div, err := NewFFIDivision(division.StartedAt, division.UpdatedAt, division.LatestValue, division.PrevValue)
		if err != nil {
			return nil, err
		}
		ffidivisions[i] = div
	}
	return ffidivisions, nil
}

func CompressedMovingAverage(latestRemovedDivision *C.struct_FFIDivision, divisions []C.struct_FFIDivision, divisionSize, windowSize, blockTime uint64) (osmomath.Dec, error) {
	var divisionsPtr *C.struct_FFIDivision
	if len(divisions) > 0 {
		divisionsPtr = &divisions[0]
	}

	result := C.compressed_moving_average(
		latestRemovedDivision,
		divisionsPtr,
		C.uintptr_t(len(divisions)),
		C.uint64_t(divisionSize),
		C.uint64_t(windowSize),
		C.uint64_t(blockTime),
	)

	errPtr := unsafe.Pointer(result.err)
	okPtr := unsafe.Pointer(result.ok)
	defer C.free(errPtr)
	defer C.free(okPtr)

	if result.err != nil {
		return osmomath.Dec{}, errors.New(C.GoString(result.err))
	}

	// CONTRACT: result.ok must not be nil if result.err is nil
	return FFIDecimalToDec(*result.ok), nil
}
