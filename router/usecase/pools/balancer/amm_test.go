package balancer

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSolveConstantFunctionInvariant(t *testing.T) {
	tests := []struct {
		name                    string
		tokenBalanceFixedBefore float64
		tokenBalanceFixedAfter  float64
		tokenWeightFixed        float64
		tokenBalanceUnknown     float64
		tokenWeightUnknown      float64
		expectedResult          float64
		expectPanic             bool
	}{
		{
			name:                    "change",
			tokenBalanceFixedBefore: 386971117259,
			tokenBalanceFixedAfter:  386971331294,
			tokenWeightFixed:        536870912000000,
			tokenBalanceUnknown:     7773284087995,
			tokenWeightUnknown:      536870912000000,
			expectedResult:          4299426.663496108929920041,
			expectPanic:             false,
		},
		{
			name:                    "no change",
			tokenBalanceFixedBefore: 100,
			tokenBalanceFixedAfter:  100,
			tokenWeightFixed:        1,
			tokenBalanceUnknown:     100,
			tokenWeightUnknown:      1,
			expectedResult:          0,
			expectPanic:             false,
		},
		{
			name:                    "panic on zero weight",
			tokenBalanceFixedBefore: 100,
			tokenBalanceFixedAfter:  90,
			tokenWeightFixed:        0.5,
			tokenBalanceUnknown:     200,
			tokenWeightUnknown:      0,
			expectPanic:             true,
		},
		{
			name:                    "overflow handling",
			tokenBalanceFixedBefore: 1000000000000000000000000000000,
			tokenBalanceFixedAfter:  1,
			tokenWeightFixed:        1000000000000000000000000000000,
			tokenBalanceUnknown:     1000000000000000000000000000000,
			tokenWeightUnknown:      1,
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
				require.InDelta(t, tc.expectedResult, result, 0.001) // Allow small floating point errors
			}
		})
	}
}

func BenchmarkSolveConstantFunctionInvariant(b *testing.B) {
	for i := 0; i < b.N; i++ {
		solveConstantFunctionInvariant(
			386971117259,
			386971331294,
			536870912000000,
			7773284087995,
			536870912000000,
		)
	}
}
