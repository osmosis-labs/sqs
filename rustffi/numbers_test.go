package rustffi_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/rustffi"
	"gotest.tools/assert"
)

func TestFFINewU128(t *testing.T) {
	testCases := []struct {
		name          string
		input         *big.Int
		expectedError error
	}{
		{
			name:  "Zero",
			input: big.NewInt(0),
		},
		{
			name:  "Small positive number",
			input: big.NewInt(42),
		},
		{
			name:  "Large number within uint64",
			input: new(big.Int).SetUint64(18446744073709551615), // 2^64 - 1
		},
		{
			name: "Number larger than uint64",
			input: func() *big.Int {
				v, _ := new(big.Int).SetString("18446744073709551616", 10) // 2^64
				return v
			}(),
		},
		{
			name: "Max u128",
			input: func() *big.Int {
				v, _ := new(big.Int).SetString("340282366920938463463374607431768211455", 10) // 2^128 - 1
				return v
			}(),
		},
		{
			name: "Max u128 + 1",
			input: func() *big.Int {
				v, _ := new(big.Int).SetString("340282366920938463463374607431768211456", 10) // 2^128
				return v
			}(),
			expectedError: errors.New("340282366920938463463374607431768211456 is too large to fit in U128"),
		},
		{
			name:          "Negative number",
			input:         big.NewInt(-1),
			expectedError: errors.New("negative number is not supported"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := rustffi.NewFFIU128(tc.input)

			if tc.expectedError == nil {
				assert.NilError(t, err)
			} else {
				assert.ErrorContains(t, err, tc.expectedError.Error())
				return
			}

			// Test conversion back to big.Int
			backToBigInt := rustffi.FFIU128ToBigInt(result)
			if backToBigInt.Cmp(tc.input) != 0 {
				t.Errorf("FFIU128ToBigInt(NewFFIU128(%v)) = %v, want %v", tc.input, backToBigInt, tc.input)
			}
		})
	}
}

func TestNewDecimal(t *testing.T) {
	testCases := []struct {
		name          string
		input         osmomath.Dec
		expectedError error
	}{
		{
			name:  "Zero",
			input: osmomath.NewDec(0),
		},
		{
			name:  "One",
			input: osmomath.NewDec(1),
		},
		{
			name:  "Large positive number",
			input: osmomath.NewDec(1000000000000000000),
		},
		{
			name:  "Fractional number",
			input: osmomath.NewDecWithPrec(123456789, 9), // 0.123456789
		},
		{
			name:          "Negative one",
			input:         osmomath.NewDec(-1),
			expectedError: errors.New("negative number is not supported"),
		},
		{
			name:          "Large negative number",
			input:         osmomath.NewDec(-1000000000000000000),
			expectedError: errors.New("negative number is not supported"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := rustffi.NewFFIDecimal(&tc.input)

			if tc.expectedError == nil {
				assert.NilError(t, err)

				// Test conversion back to osmomath.Dec
				backToDec := rustffi.FFIDecimalToDec(result)
				if !backToDec.Equal(tc.input) {
					t.Errorf("FFIDecimalToDec(NewFFIDecimal(%v)) = %v, want %v", tc.input, backToDec, tc.input)
				}
			} else {
				assert.ErrorContains(t, err, tc.expectedError.Error())
			}
		})
	}
}
