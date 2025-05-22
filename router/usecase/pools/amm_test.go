package pools

import (
	"math/big"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/stretchr/testify/require"
)

func TestSolveConstantFunctionInvariant(t *testing.T) {
	tests := []struct {
		name                    string
		tokenBalanceFixedBefore osmomath.Dec
		tokenBalanceFixedAfter  osmomath.Dec
		tokenWeightFixed        osmomath.Dec
		tokenBalanceUnknown     osmomath.Dec
		tokenWeightUnknown      osmomath.Dec
		expectedResult          osmomath.Dec
		expectPanic             bool
	}{
		{
			name:                    "change",
			tokenBalanceFixedBefore: osmomath.NewDec(386971117259),
			tokenBalanceFixedAfter:  osmomath.NewDec(386971331294),
			tokenWeightFixed:        osmomath.NewDec(536870912000000),
			tokenBalanceUnknown:     osmomath.NewDec(7773284087995),
			tokenWeightUnknown:      osmomath.NewDec(536870912000000),
			expectedResult:          osmomath.MustNewDecFromStr("4299426.663416173517055000"),
			expectPanic:             false,
		},
		{
			name:                    "no change",
			tokenBalanceFixedBefore: osmomath.NewDec(100),
			tokenBalanceFixedAfter:  osmomath.NewDec(100),
			tokenWeightFixed:        osmomath.NewDec(1),
			tokenBalanceUnknown:     osmomath.NewDec(100),
			tokenWeightUnknown:      osmomath.NewDec(1),
			expectedResult:          osmomath.NewDec(0),
			expectPanic:             false,
		},
		{
			name:                    "panic on zero weight",
			tokenBalanceFixedBefore: osmomath.NewDec(100),
			tokenBalanceFixedAfter:  osmomath.NewDec(90),
			tokenWeightFixed:        osmomath.NewDecWithPrec(5, 1),
			tokenBalanceUnknown:     osmomath.NewDec(200),
			tokenWeightUnknown:      osmomath.NewDec(0), // Should panic
			expectPanic:             true,
		},
		{
			name:                    "overflow handling",
			tokenBalanceFixedBefore: osmomath.MustNewDecFromStr("1000000000000000000000000000000"),
			tokenBalanceFixedAfter:  osmomath.NewDec(1),
			tokenWeightFixed:        osmomath.MustNewDecFromStr("1000000000000000000000000000000"),
			tokenBalanceUnknown:     osmomath.MustNewDecFromStr("1000000000000000000000000000000"),
			tokenWeightUnknown:      osmomath.NewDec(1),
			expectPanic:             true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic but did not get one")
					}
				}()
			}

			result := solveConstantFunctionInvariant(
				tc.tokenBalanceFixedBefore,
				tc.tokenBalanceFixedAfter,
				tc.tokenWeightFixed,
				tc.tokenBalanceUnknown,
				tc.tokenWeightUnknown,
			)

			if !tc.expectPanic {
				require.Equal(t, tc.expectedResult.String(), result.String())
			}
		})
	}
}

func mustBigFloat(s string) *big.Float {
	f, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil {
		panic(err)
	}
	return f
}

func TestSolveConstantFunctionInvariant2(t *testing.T) {
	tests := []struct {
		name                    string
		tokenBalanceFixedBefore *big.Float
		tokenBalanceFixedAfter  *big.Float
		tokenWeightFixed        *big.Float
		tokenBalanceUnknown     *big.Float
		tokenWeightUnknown      *big.Float
		expectedResult          *big.Float
		expectPanic             bool
	}{
		{
			name:                    "change",
			tokenBalanceFixedBefore: mustBigFloat("386971117259"),
			tokenBalanceFixedAfter:  mustBigFloat("386971331294"),
			tokenWeightFixed:        mustBigFloat("536870912000000"),
			tokenBalanceUnknown:     mustBigFloat("7773284087995"),
			tokenWeightUnknown:      mustBigFloat("536870912000000"),
			expectedResult:          mustBigFloat("4299426.663496108929920041"),
			expectPanic:             false,
		},
		{
			name:                    "no change",
			tokenBalanceFixedBefore: mustBigFloat("100"),
			tokenBalanceFixedAfter:  mustBigFloat("100"),
			tokenWeightFixed:        mustBigFloat("1"),
			tokenBalanceUnknown:     mustBigFloat("100"),
			tokenWeightUnknown:      mustBigFloat("1"),
			expectedResult:          mustBigFloat("0"),
			expectPanic:             false,
		},
		{
			name:                    "panic on zero weight",
			tokenBalanceFixedBefore: mustBigFloat("100"),
			tokenBalanceFixedAfter:  mustBigFloat("90"),
			tokenWeightFixed:        mustBigFloat("0.5"),
			tokenBalanceUnknown:     mustBigFloat("200"),
			tokenWeightUnknown:      mustBigFloat("0"),
			expectPanic:             true,
		},
		{
			name:                    "overflow handling",
			tokenBalanceFixedBefore: mustBigFloat("1000000000000000000000000000000"),
			tokenBalanceFixedAfter:  mustBigFloat("1"),
			tokenWeightFixed:        mustBigFloat("1000000000000000000000000000000"),
			tokenBalanceUnknown:     mustBigFloat("1000000000000000000000000000000"),
			tokenWeightUnknown:      mustBigFloat("1"),
			expectPanic:             true,
		},
	}

	for _, tc := range tests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			if tc.expectPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Errorf("expected panic but did not get one")
					}
				}()
			}
			result := solveConstantFunctionInvariantBigFloat(
				tc.tokenBalanceFixedBefore,
				tc.tokenBalanceFixedAfter,
				tc.tokenWeightFixed,
				tc.tokenBalanceUnknown,
				tc.tokenWeightUnknown,
			)

			if !tc.expectPanic {
				require.Equal(t, tc.expectedResult.String(), result.String())
			}
		})
	}
}

func BenchmarkSolveConstantFunctionInvariant(b *testing.B) {
	for i := 0; i < b.N; i++ {
		solveConstantFunctionInvariant(
			osmomath.NewDec(386971117259),
			osmomath.NewDec(386971331294),
			osmomath.NewDec(536870912000000),
			osmomath.NewDec(7773284087995),
			osmomath.NewDec(536870912000000),
		)
	}
}

func BenchmarkSolveConstantFunctionInvariant2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		solveConstantFunctionInvariantBigFloat(
			big.NewFloat(386971117259),
			big.NewFloat(386971331294),
			big.NewFloat(536870912000000),
			big.NewFloat(7773284087995),
			big.NewFloat(536870912000000),
		)
	}
}
