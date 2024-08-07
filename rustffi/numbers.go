package rustffi

/*
#include "../target/release/libsqs_ffi.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"math/big"

	"github.com/osmosis-labs/osmosis/osmomath"
)

func NewFFIDecimal(d *osmomath.Dec) (C.struct_FFIDecimal, error) {
	u128, err := NewFFIU128(d.BigInt())
	if err != nil {
		return C.struct_FFIDecimal{}, err
	}
	return C.struct_FFIDecimal{_0: u128}, nil
}

func FFIDecimalToDec(d C.struct_FFIDecimal) osmomath.Dec {
	return osmomath.NewDecFromBigIntWithPrec(FFIU128ToBigInt(d._0), osmomath.DecPrecision)
}

func NewFFIU128(i *big.Int) (C.struct_FFIU128, error) {
	if i.Sign() < 0 {
		return C.struct_FFIU128{}, errors.New("negative number is not supported")
	}

	bits := i.Bits()
	u128Bits := [2]C.ulonglong{}

	if len(bits) == 0 {
		u128Bits[0] = C.ulonglong(0)
		u128Bits[1] = C.ulonglong(0)
	} else if len(bits) == 1 {
		u128Bits[0] = C.ulonglong(bits[0])
		u128Bits[1] = C.ulonglong(0)
	} else if len(bits) == 2 {
		u128Bits[0] = C.ulonglong(bits[0])
		u128Bits[1] = C.ulonglong(bits[1])
	} else {
		return C.struct_FFIU128{}, fmt.Errorf("%d is too large to fit in U128", i)
	}
	return C.struct_FFIU128{_0: u128Bits}, nil
}

func FFIU128ToBigInt(u128 C.struct_FFIU128) *big.Int {
	bits := [2]big.Word{}
	bits[0] = big.Word(u128._0[0])
	bits[1] = big.Word(u128._0[1])
	return big.NewInt(0).SetBits(bits[:])
}
