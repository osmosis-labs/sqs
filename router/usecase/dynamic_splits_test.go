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

// Sanity check that the exact-out (in-given-out) split optimizer returns a non-nil quote
// for a real mainnet pair, and that the split outputs sum to exactly the requested output.
func (s *RouterTestSuite) TestGetSplitQuoteInGivenOut() {
	const displayDenomOut = "pepe"
	var amountOut = osmomath.NewInt(9_000_000_000_000_000_000)

	tokenOut, rankedRoutes := s.setupSplitsInGivenOutMainnetTestCase(displayDenomOut, amountOut, USDC)

	splitQuote, err := usecase.GetSplitQuoteInGivenOut(context.TODO(), rankedRoutes, tokenOut)

	s.Require().NoError(err)
	s.Require().NotNil(splitQuote)

	// Exact-out invariant: the user must receive exactly the requested output.
	s.Require().Equal(tokenOut.Amount.String(), splitQuote.GetAmountOut().Amount.String())

	// The per-route outputs must sum to exactly the requested output, and the per-route inputs
	// to the reported total input.
	sumOut := osmomath.ZeroInt()
	sumIn := osmomath.ZeroInt()
	for _, r := range splitQuote.GetRoute() {
		sumOut = sumOut.Add(r.GetAmountOut())
		sumIn = sumIn.Add(r.GetAmountIn())
	}
	s.Require().Equal(tokenOut.Amount.String(), sumOut.String(), "split outputs must sum to the requested output")
	s.Require().Equal(splitQuote.GetAmountIn().Amount.String(), sumIn.String(), "split inputs must sum to the reported total input")
}

// TestGetSplitQuoteInGivenOut_RouteErrorHandling tests that, on the exact-out path, routes
// returning errors (e.g. orderbook pools with insufficient liquidity) are excluded from the
// split optimization rather than causing the whole quote to fail.
func (s *RouterTestSuite) TestGetSplitQuoteInGivenOut_RouteErrorHandling() {
	const (
		tokenInDenom  = "uusdc"
		tokenOutDenom = "ubtc"
	)

	// A pool that always fills exact-out with a 1:1 input requirement.
	goodPool := &mocks.MockRoutablePool{
		ID:            1,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      osmomath.ZeroDec(),
		SpreadFactor:  osmomath.ZeroDec(),
		CalculateTokenInByTokenOutFunc: func(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
			// 1:1 input for output.
			return sdk.NewCoin(tokenInDenom, tokenOut.Amount), nil
		},
	}

	// A pool that errors (simulating an orderbook with insufficient liquidity).
	errorPool := &mocks.MockRoutablePool{
		ID:            2,
		TokenOutDenom: tokenOutDenom,
		TakerFee:      osmomath.ZeroDec(),
		SpreadFactor:  osmomath.ZeroDec(),
		CalculateTokenInByTokenOutFunc: func(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
			return sdk.Coin{}, errors.New("orderbook: not enough liquidity to complete swap")
		},
	}

	routes := []route.RouteImpl{
		{Pools: []domain.RoutablePool{goodPool}},
		{Pools: []domain.RoutablePool{errorPool}},
	}

	tokenOut := sdk.NewCoin(tokenOutDenom, osmomath.NewInt(1_000_000))

	splitQuote, err := usecase.GetSplitQuoteInGivenOut(context.TODO(), routes, tokenOut)

	// The erroring route should be excluded, not fail the quote; all output is filled by the
	// good route, so the required input is positive and the output matches the request.
	s.Require().NoError(err)
	s.Require().NotNil(splitQuote)
	s.Require().Equal(tokenOut.Amount.String(), splitQuote.GetAmountOut().Amount.String())
	s.Require().True(splitQuote.GetAmountIn().Amount.IsPositive(),
		"expected positive input from the good route, got %s", splitQuote.GetAmountIn().String())
}

// TestGetSplitQuoteInGivenOut_NotWorseThanSingleRoute is the core financial-correctness
// check: for the same desired output, the split optimizer must require no more input than the
// single cheapest route. (Splitting can only help or tie; it must never make the user pay
// more.)
func (s *RouterTestSuite) TestGetSplitQuoteInGivenOut_NotWorseThanSingleRoute() {
	const displayDenomOut = "pepe"
	var amountOut = osmomath.NewInt(9_000_000_000_000_000_000)

	tokenOut, rankedRoutes := s.setupSplitsInGivenOutMainnetTestCase(displayDenomOut, amountOut, USDC)
	s.Require().NotEmpty(rankedRoutes)

	splitQuote, err := usecase.GetSplitQuoteInGivenOut(context.TODO(), rankedRoutes, tokenOut)
	s.Require().NoError(err)

	// Best single route input: minimum input across all ranked routes for the full output.
	bestSingleIn := osmomath.Int{}
	for _, r := range rankedRoutes {
		coinIn, err := r.CalculateTokenInByTokenOut(context.TODO(), tokenOut)
		if err != nil {
			continue
		}
		if bestSingleIn.IsNil() || coinIn.Amount.LT(bestSingleIn) {
			bestSingleIn = coinIn.Amount
		}
	}
	s.Require().False(bestSingleIn.IsNil(), "expected at least one fillable single route")

	// The split must require no more input than the best single route.
	s.Require().True(splitQuote.GetAmountIn().Amount.LTE(bestSingleIn),
		"split input %s must be <= best single-route input %s",
		splitQuote.GetAmountIn().String(), bestSingleIn.String())
}

// setupSplitsInGivenOutMainnetTestCase mirrors setupSplitsMainnetTestCase for the exact-out
// (in-given-out) direction: it runs candidate search and ranking up to the point where split
// computation begins, and returns the desired tokenOut coin and the ranked routes.
func (s *RouterTestSuite) setupSplitsInGivenOutMainnetTestCase(displayDenomOut string, amountOut osmomath.Int, chainDenomIn string) (sdk.Coin, []route.RouteImpl) {
	mainnetState := s.SetupMainnetState()
	useCases := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithLoggerDisabled())

	chainDenomOut, err := useCases.Tokens.GetChainDenom(displayDenomOut)
	s.Require().NoError(err)

	tokenOut := sdk.NewCoin(chainDenomOut, amountOut)

	ctx := context.TODO()
	config := useCases.Router.GetConfig()

	options := domain.CandidateRouteSearchOptions{
		MaxRoutes:           config.MaxRoutes,
		MaxPoolsPerRoute:    config.MaxPoolsPerRoute,
		MinPoolLiquidityCap: config.MinPoolLiquidityCap,
	}

	candidateRoutes, err := useCases.CandidateRouteSearcher.FindCandidateRoutesInGivenOut(context.Background(), tokenOut, chainDenomIn, options)
	s.Require().NoError(err)

	routerUseCase, ok := useCases.Router.(*usecase.RouterUseCaseImpl)
	s.Require().True(ok)

	_, rankedRoutes, err := routerUseCase.RankRoutesByDirectQuoteInGivenOut(ctx, candidateRoutes, tokenOut, chainDenomIn, config.MaxRoutes)
	s.Require().NoError(err)

	return tokenOut, rankedRoutes
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
