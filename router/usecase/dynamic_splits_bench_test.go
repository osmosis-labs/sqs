package usecase_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase"
)

// Microbenchmark for the GetSplitQuote function.
func BenchmarkGetSplitQuote(b *testing.B) {
	// This is a hack to be able to use test suite helpers with the benchmark.
	// We need to set testing.T for assertings within the helpers. Otherwise, it would block
	s := RouterTestSuite{}
	s.SetT(&testing.T{})

	const displayDenomIn = "pepe"
	var (
		amountIn = osmomath.NewInt(9_000_000_000_000_000_000)
		tokenIn  = sdk.NewCoin(displayDenomIn, amountIn)
	)

	tokenIn, rankedRoutes := s.setupSplitsMainnetTestCase(displayDenomIn, amountIn, USDC)

	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// System under test.
		_, err := usecase.GetSplitQuote(context.TODO(), rankedRoutes, tokenIn, domain.TokenSwapMethodExactIn)
		s.Require().NoError(err)
		if err != nil {
			b.Errorf("GetPrices returned an error: %v", err)
		}
	}
}
