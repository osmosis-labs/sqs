package usecase_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
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

	splitQuote, err := usecase.GetSplitQuote(context.TODO(), rankedRoutes, tokenIn, domain.TokenSwapMethodExactIn)

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

	config := useCases.Router.GetConfig()

	// TODO: consider moving to interface.
	routerUseCase, ok := useCases.Router.(*usecase.RouterUseCaseImpl)
	s.Require().True(ok)

	// Estimate direct quote
	_, rankedRoutes, err := routerUseCase.RankRoutesByDirectQuote(ctx, candidateRoutes, tokenIn, chainDenomOut, domain.TokenSwapMethodExactIn, config.MaxRoutes)
	s.Require().NoError(err)

	return tokenIn, rankedRoutes
}
