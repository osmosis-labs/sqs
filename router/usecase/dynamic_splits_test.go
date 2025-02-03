package usecase_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"

	"github.com/stretchr/testify/suite"
)

type DynamicSplitsTestSuite struct {
	routertesting.RouterTestHelper
}

func TestDynamicSplitsTestSuite(t *testing.T) {
	suite.Run(t, new(DynamicSplitsTestSuite))
}

// Sanity check test case to validate get split quote function with a given denom and amount.
// This test case tests OutGivenIn swap method.
func (s *DynamicSplitsTestSuite) TestGetSplitQuoteOutGivenIn() {
	const displayDenomIn = "pepe"
	var (
		amountIn = osmomath.NewInt(9_000_000_000_000_000_000)
		tokenIn  = sdk.NewCoin(displayDenomIn, amountIn)
	)

	tokenIn, rankedRoutes := s.setupSplitsMainnetTestCaseOutGivenIn(displayDenomIn, amountIn, USDC)

	splitQuote, err := usecase.GetSplitQuoteOutGivenIn(context.TODO(), rankedRoutes, tokenIn)

	s.Require().NotNil(splitQuote)
	s.Require().NoError(err)
}


// Sanity check test case to validate get split quote function with a given denom and amount.
// This test case tests InGivenOut swap method.
func (s *DynamicSplitsTestSuite) TestGetSplitQuoteInGivenOut() {
	const displayDenomIn = "pepe"
	var (
		amountIn = osmomath.NewInt(9_000_000_000_000_000_000)
		tokenIn  = sdk.NewCoin(displayDenomIn, amountIn)
	)

	tokenIn, rankedRoutes := s.setupSplitsMainnetTestCaseInGivenOut(displayDenomIn, amountIn, USDC)

	splitQuote, err := usecase.GetSplitQuoteInGivenOut(context.TODO(), rankedRoutes, tokenIn)

	s.Require().NotNil(splitQuote)
	s.Require().NoError(err)
}

// setupSplitsMainnetTestCaseOutGivenIn sets up the test case for GetSplitQuote using mainnet state.
// Calls all the relevant functions as if we were estimating the quote up until starting the
// splits computation.
//
// Utilizes the given display denom in, amount in and chain denom out.
func (s *DynamicSplitsTestSuite) setupSplitsMainnetTestCaseOutGivenIn(displayDenomIn string, amountIn osmomath.Int, chainDenomOut string) (sdk.Coin, []route.RouteImpl) {
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
	candidateRoutes, err := useCases.CandidateRouteSearcher.FindCandidateRoutesOutGivenIn(tokenIn, chainDenomOut, options)
	s.Require().NoError(err)

	// TODO: consider moving to interface.
	routerUseCase, ok := useCases.Router.(*usecase.RouterUseCaseImpl)
	s.Require().True(ok)

	// Estimate direct quote
	_, rankedRoutes, err := routerUseCase.RankRoutesByDirectQuoteOutGivenIn(ctx, candidateRoutes, tokenIn, chainDenomOut, config.MaxRoutes)
	s.Require().NoError(err)

	return tokenIn, rankedRoutes
}

// setupSplitsMainnetTestCaseInGivenOut sets up the test case for GetSplitQuote using mainnet state.
// Calls all the relevant functions as if we were estimating the quote up until starting the
// splits computation.
//
// Utilizes the given display denom in, amount in and chain denom out.
func (s *DynamicSplitsTestSuite) setupSplitsMainnetTestCaseInGivenOut(displayDenomOut string, amountOut osmomath.Int, chainDenomIn string) (sdk.Coin, []route.RouteImpl) {
	// Setup mainnet state
	mainnetState := s.SetupMainnetState()

	// Setup router and pools use case.
	useCases := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithLoggerDisabled())

	// Translate display denom to chain denom
	chainDenom, err := useCases.Tokens.GetChainDenom(displayDenomOut)
	s.Require().NoError(err)

	tokenOut := sdk.NewCoin(chainDenom, amountOut)

	ctx := context.TODO()

	config := useCases.Router.GetConfig()

	options := domain.CandidateRouteSearchOptions{
		MaxRoutes:           config.MaxRoutes,
		MaxPoolsPerRoute:    config.MaxPoolsPerRoute,
		MinPoolLiquidityCap: config.MinPoolLiquidityCap,
	}
	// Get candidate routes
	candidateRoutes, err := useCases.CandidateRouteSearcher.FindCandidateRoutesInGivenOut(tokenOut, chainDenomIn, options)
	s.Require().NoError(err)

	// TODO: consider moving to interface.
	routerUseCase, ok := useCases.Router.(*usecase.RouterUseCaseImpl)
	s.Require().True(ok)

	// Estimate direct quote
	_, rankedRoutes, err := routerUseCase.RankRoutesByDirectQuoteInGivenOut(ctx, candidateRoutes, tokenOut, chainDenomIn, config.MaxRoutes)
	s.Require().NoError(err)

	return tokenOut, rankedRoutes
}
