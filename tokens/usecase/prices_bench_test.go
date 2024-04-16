package usecase_test

import (
	"context"
	"testing"

	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
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
	mainnetState := s.SetupMainnetState()

	// Customize cache config
	mainnetState.PricingConfig.CacheExpiryMs = pricingCacheExpiry

	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// System under test.
		_, err := mainnetUsecase.Tokens.GetPrices(context.Background(), routertesting.MainnetDenoms, []string{USDC, USDT})
		s.Require().NoError(err)
		if err != nil {
			b.Errorf("GetPrices returned an error: %v", err)
		}
	}
}
