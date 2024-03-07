package usecase_test

import (
	"context"
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
)

const (
	pricingCacheExpiry = 2000
)

func BenchmarkGetPrices(b *testing.B) {
	// This is a hack to be able to use test suite helpers with the benchmark.
	// We need to set testing.T for assertings within the helpers. Otherwise, it would block
	s := routertesting.RouterTestHelper{}
	s.SetT(&testing.T{})

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState, cache.New(), cache.New())

	// Set up on-chain pricing strategy
	pricingConfig := domain.PricingConfig{
		DefaultSource: domain.ChainPricingSource,
		CacheExpiryMs: pricingCacheExpiry,
	}
	pricingStrategy, err := pricing.NewPricingStrategy(pricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// System under test.
		_, err := mainnetUsecase.Tokens.GetPrices(context.Background(), routertesting.MainnetDenoms, []string{USDC, USDT}, pricingStrategy)
		s.Require().NoError(err)
		if err != nil {
			b.Errorf("GetPrices returned an error: %v", err)
		}
	}
}
