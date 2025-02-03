package usecase_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/router/usecase"
)

// Microbenchmark for the GetSplitQuoteOutGivenIn  function.
func BenchmarkGetSplitQuoteOutGivenIn(b *testing.B) {
	// This is a hack to be able to use test suite helpers with the benchmark.
	// We need to set testing.T for assertings within the helpers. Otherwise, it would block
	s := DynamicSplitsTestSuite{}
	s.SetT(&testing.T{})

	const displayDenomIn = "pepe"
	var (
		amountIn = osmomath.NewInt(9_000_000_000_000_000_000)
		tokenIn  = sdk.NewCoin(displayDenomIn, amountIn)
	)

	tokenIn, rankedRoutes := s.setupSplitsMainnetTestCaseOutGivenIn(displayDenomIn, amountIn, USDC)

	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// System under test.
		_, err := usecase.GetSplitQuoteOutGivenIn(context.TODO(), rankedRoutes, tokenIn)
		s.Require().NoError(err)
		if err != nil {
			b.Errorf("GetPrices returned an error: %v", err)
		}
	}
}

// Microbenchmark for the GetSplitQuoteOutGivenIn  function.
func BenchmarkGetSplitQuoteInGivenOut(b *testing.B) {
	// This is a hack to be able to use test suite helpers with the benchmark.
	// We need to set testing.T for assertings within the helpers. Otherwise, it would block
	s := DynamicSplitsTestSuite{}
	s.SetT(&testing.T{})

	const displayDenomOut = "pepe"
	var (
		amountOut = osmomath.NewInt(9_000_000_000_000_000_000)
		tokenOut  = sdk.NewCoin(displayDenomOut, amountOut)
	)

	tokenOut, rankedRoutes := s.setupSplitsMainnetTestCaseInGivenOut(displayDenomOut, amountOut, USDC)

	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// System under test.
		_, err := usecase.GetSplitQuoteInGivenOut(context.TODO(), rankedRoutes, tokenOut)
		s.Require().NoError(err)
		if err != nil {
			b.Errorf("GetPrices returned an error: %v", err)
		}
	}
}
