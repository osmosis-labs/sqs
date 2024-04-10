package usecase_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/router/usecase"
)

var (
	testAmount = osmomath.NewInt(1234567890323344555)
	TenE9      = osmomath.NewInt(1_000_000_000)
	TenE8      = osmomath.NewInt(100_000_000)
	TenE7      = osmomath.NewInt(10_000_000)
	TenE6      = osmomath.NewInt(1_000_000)
	TenE5      = osmomath.NewInt(100_000)
	TenE4      = osmomath.NewInt(10_000)
	TenE3      = osmomath.NewInt(1_000)
	TenE2      = osmomath.NewInt(100)
	TenE1      = osmomath.NewInt(10)
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
			amount: TenE9.Sub(osmomath.OneInt()),
		},
		"10^9": {
			amount: TenE9,
		},
		"10^9 +1": {
			amount: TenE9.Add(osmomath.OneInt()),
		},
		"10^18 +1": {
			amount: TenE9.Mul(TenE9).Add(osmomath.OneInt()),
		},
		"10^15 +5": {
			amount: TenE9.Mul(TenE6).Add(osmomath.OneInt()),
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
