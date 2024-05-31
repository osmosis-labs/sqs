package usecase_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

// Sanity check test case to validate get split quote function with a given denom and amount.
func (s *RouterTestSuite) TestGetSplitQuote() {
	const displayDenomIn = "pepe"
	var (
		amountIn = osmomath.NewInt(9_000_000_000_000_000_000)
		tokenIn  = sdk.NewCoin(displayDenomIn, amountIn)
	)

	tokenIn, rankedRoutes := s.setupSplitsMainnetTestCase(displayDenomIn, amountIn, USDC)

	splitQuote, err := usecase.GetSplitQuote(context.TODO(), rankedRoutes, tokenIn)

	s.Require().NotNil(splitQuote)
	s.Require().NoError(err)
}

// setupSplitsMainnetTestCase sets up the test case for GetSplitQuote using mainnet state.
// Calls all the relevant functions as if we were estimating the quote up until starting the
// splits computation.
//
// Utilizes the given display denom in, amount in and chain denom out.
func (s *RouterTestSuite) setupSplitsMainnetTestCase(displayDenomIn string, amountIn osmomath.Int, chainDenomOut string) (sdk.Coin, []route.RouteImpl) {
	// Setup mainnet state
	mainnetState := s.SetupMainnetState()

	// Setup router and pools use case.
	useCases := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithLoggerDisabled())

	// Translate display denom to chain denom
	chainDenom, err := useCases.Tokens.GetChainDenom(displayDenomIn)
	s.Require().NoError(err)

	tokenIn := sdk.NewCoin(chainDenom, amountIn)

	ctx := context.TODO()

	// Get candidate routes
	candidateRoutes, err := useCases.Router.GetCandidateRoutes(ctx, tokenIn, chainDenomOut)
	s.Require().NoError(err)

	// Calculate routes from candidate routes
	routes, err := useCases.Pools.GetRoutesFromCandidates(candidateRoutes, tokenIn.Denom, chainDenomOut)
	s.Require().NoError(err)

	config := useCases.Router.GetConfig()

	// Estimate direct quote
	_, rankedRoutes, err := usecase.EstimateDirectQuote(ctx, routes, tokenIn, config.MaxRoutes, &log.NoOpLogger{})
	s.Require().NoError(err)

	rankedRoutes = rankedRoutes[:config.MaxSplitRoutes]

	return tokenIn, rankedRoutes
}
