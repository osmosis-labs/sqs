package usecase_test

import (
	"context"
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
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

// TestGetSplitQuote_RouteErrorHandling tests that routes returning errors
// (e.g., orderbook pools with insufficient liquidity) are properly excluded
// from the split quote optimization, rather than silently failing.
func (s *RouterTestSuite) TestGetSplitQuote_RouteErrorHandling() {
	const (
		tokenInDenom  = "uusdc"
		tokenOutDenom = "ubtc"
	)

	// Create a mock pool that always succeeds with good output
	goodPool := &mocks.MockRoutablePool{
		ID:            1,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      osmomath.ZeroDec(),
		SpreadFactor:  osmomath.ZeroDec(),
		CalculateTokenOutByTokenInFunc: func(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
			// Returns 1:1 ratio for simplicity
			return sdk.NewCoin(tokenOutDenom, tokenIn.Amount), nil
		},
	}

	// Create a mock pool that returns an error (simulating orderbook with insufficient liquidity)
	errorPool := &mocks.MockRoutablePool{
		ID:            2,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      osmomath.ZeroDec(),
		SpreadFactor:  osmomath.ZeroDec(),
		CalculateTokenOutByTokenInFunc: func(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
			return sdk.Coin{}, errors.New("orderbook: not enough liquidity to complete swap")
		},
	}

	// Create routes: one good route and one that errors
	goodRoute := route.RouteImpl{
		Pools: []domain.RoutablePool{goodPool},
	}
	errorRoute := route.RouteImpl{
		Pools: []domain.RoutablePool{errorPool},
	}

	routes := []route.RouteImpl{goodRoute, errorRoute}

	// Test with an amount
	tokenIn := sdk.NewCoin(tokenInDenom, osmomath.NewInt(1_000_000))

	// Get split quote - should succeed using only the good route
	splitQuote, err := usecase.GetSplitQuote(context.TODO(), routes, tokenIn)

	// Should not error - the erroring route should be excluded, not cause failure
	s.Require().NoError(err)
	s.Require().NotNil(splitQuote)

	// The output should be reasonable (from the good route only)
	// Since the error route returns zero, all traffic should go through the good route
	s.Require().True(splitQuote.GetAmountOut().Amount.GT(osmomath.ZeroInt()),
		"Expected positive output from good route, got zero")
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

	config := useCases.Router.GetConfig()

	options := domain.CandidateRouteSearchOptions{
		MaxRoutes:           config.MaxRoutes,
		MaxPoolsPerRoute:    config.MaxPoolsPerRoute,
		MinPoolLiquidityCap: config.MinPoolLiquidityCap,
	}
	// Get candidate routes
	candidateRoutes, err := useCases.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(context.Background(), tokenIn, chainDenomOut, options)
	s.Require().NoError(err)

	// TODO: consider moving to interface.
	routerUseCase, ok := useCases.Router.(*usecase.RouterUseCaseImpl)
	s.Require().True(ok)

	// Estimate direct quote
	_, rankedRoutes, err := routerUseCase.RankRoutesByDirectQuoteOutGivenIn(ctx, candidateRoutes, tokenIn, chainDenomOut, config.MaxRoutes)
	s.Require().NoError(err)

	return tokenIn, rankedRoutes
}
