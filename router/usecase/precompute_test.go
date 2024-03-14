package usecase_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/router/usecase"
)

var (
	testAmount = osmomath.NewInt(1234567890323344555)
)

func (s *RouterTestSuite) TestGetPrecomputeOrderOfMagnitude() {

	tests := map[string]struct {
		amount osmomath.Int
	}{
		"0 = 0": {
			amount: osmomath.ZeroInt(),
		},
		"1 = 0": {
			amount: osmomath.OneInt(),
		},
		"9.99 = 0": {
			amount: osmomath.NewInt(9),
		},
		"10^9 - 1": {
			amount: usecase.TenE9.Sub(osmomath.OneInt()),
		},
		"10^9": {
			amount: usecase.TenE9,
		},
		"10^9 +1": {
			amount: usecase.TenE9.Add(osmomath.OneInt()),
		},
		"10^18 +1": {
			amount: usecase.TenE9.Mul(usecase.TenE9).Add(osmomath.OneInt()),
		},
		"10^15 +5": {
			amount: usecase.TenE9.Mul(usecase.TenE6).Add(osmomath.OneInt()),
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {

			actual := usecase.GetPrecomputeOrderOfMagnitude(tc.amount)

			expected := osmomath.OrderOfMagnitude(tc.amount.ToLegacyDec())

			s.Require().Equal(expected, actual)
		})
	}

}

// go test -benchmem -run=^$ -bench ^BenchmarkGetPrecomputeOrderOfMagnitude$ github.com/osmosis-labs/sqs/router/usecase -count=6
func BenchmarkGetPrecomputeOrderOfMagnitude(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = usecase.GetPrecomputeOrderOfMagnitude(testAmount)
	}
}

// go test -benchmem -run=^$ -bench ^BenchmarkOrderOfMagnitude$ github.com/osmosis-labs/sqs/router/usecase -count=6 > old
func BenchmarkOrderOfMagnitude(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = osmomath.OrderOfMagnitude(testAmount.ToLegacyDec())
	}
}
