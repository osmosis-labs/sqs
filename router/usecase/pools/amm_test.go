package pools

import (
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
