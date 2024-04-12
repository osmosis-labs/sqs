package usecase_test

import (
	"fmt"
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
	type testcase struct {
		amount osmomath.Int
	}
	tests := map[string]testcase{
		"0 = 0": {
			amount: osmomath.ZeroInt(),
		},
		"1 = 0": {
			amount: osmomath.OneInt(),
		},
		"9.99 = 0": {
			amount: osmomath.NewInt(9),
		},
		"10^15 +5": {
			amount: TenE9.Mul(TenE6).Add(osmomath.OneInt()),
		},
	}
	curPowTen := osmomath.OneInt()
	for i := 1; i < 20; i++ {
		curPowTen = curPowTen.Mul(TenE1)
		tests[fmt.Sprintf("10^%d", i)] = testcase{amount: curPowTen}
		tests[fmt.Sprintf("10^%d +1", i)] = testcase{amount: curPowTen.AddRaw(1)}
		tests[fmt.Sprintf("10^%d -1", i)] = testcase{amount: curPowTen.SubRaw(1)}
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
