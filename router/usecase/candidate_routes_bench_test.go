package usecase_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

// Microbenchmark for the GetSplitQuote function.
func BenchmarkCandidateRouteSearcher(b *testing.B) {
	// This is a hack to be able to use test suite helpers with the benchmark.
	// We need to set testing.T for assertings within the helpers. Otherwise, it would block
	s := RouterTestSuite{}
	s.SetT(&testing.T{})

	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithLoggerDisabled())

	var (
		amountIn      = osmomath.NewInt(1_000_000)
		tokenIn       = sdk.NewCoin(UOSMO, amountIn)
		tokenOutDenom = ATOM
	)

	routerConfig := usecase.Router.GetConfig()
	candidateRouteOptions := domain.CandidateRouteSearchOptions{
		MaxRoutes:           routerConfig.MaxRoutes,
		MaxPoolsPerRoute:    routerConfig.MaxPoolsPerRoute,
		MinPoolLiquidityCap: 1,
	}

	b.ResetTimer()

	// Run the benchmark
	for i := 0; i < b.N; i++ {
		// System under test
		_, err := usecase.CandidateRouteSearcher.FindCandidateRoutes(tokenIn, tokenOutDenom, candidateRouteOptions)
		s.Require().NoError(err)
		if err != nil {
			b.Errorf("FindCandidateRoutes returned an error: %v", err)
		}
	}
}
