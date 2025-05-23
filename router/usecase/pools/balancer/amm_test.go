package balancer

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustBigFloat(s string) *big.Float {
	f, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil {
		panic(err)
	}
	return f
}

func TestSolveConstantFunctionInvariant(t *testing.T) {
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

func BenchmarkSolveConstantFunctionInvariant(b *testing.B) {
	for i := 0; i < b.N; i++ {
		solveConstantFunctionInvariant(
			big.NewFloat(386971117259),
			big.NewFloat(386971331294),
			big.NewFloat(536870912000000),
			big.NewFloat(7773284087995),
			big.NewFloat(536870912000000),
		)
	}
}
