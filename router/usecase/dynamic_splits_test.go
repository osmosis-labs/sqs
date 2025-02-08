package usecase_test

import (
	"context"
	"math"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type DynamicSplitsTestSuite struct {
	routertesting.RouterTestHelper
}

func TestDynamicSplitsTestSuite(t *testing.T) {
	suite.Run(t, new(DynamicSplitsTestSuite))
}

var zero = osmomath.NewInt(0)
var inf = osmomath.NewInt(math.MaxInt64)

func TestMin(t *testing.T) {
	testCases := []struct {
		name                string
		totalRoutes         int
		profitFunc          usecase.ProfitFunc
		expectedDP          [][]osmomath.Int
		expectedProportions [][]uint8
	}{
		{
			name:        "Single route",
			totalRoutes: 1,
			profitFunc: func(route int, proportion uint8) osmomath.Int {
				return osmomath.NewInt(int64(proportion))
			},
			expectedDP: [][]osmomath.Int{
				{zero, zero},
				{inf, osmomath.NewInt(1)},
				{inf, osmomath.NewInt(2)},
				{inf, osmomath.NewInt(3)},
				{inf, osmomath.NewInt(4)},
				{inf, osmomath.NewInt(5)},
				{inf, osmomath.NewInt(6)},
				{inf, osmomath.NewInt(7)},
				{inf, osmomath.NewInt(8)},
				{inf, osmomath.NewInt(9)},
				{inf, osmomath.NewInt(10)},
			},
			expectedProportions: [][]uint8{
				{0, 0},
				{0, 1},
				{0, 2},
				{0, 3},
				{0, 4},
				{0, 5},
				{0, 6},
				{0, 7},
				{0, 8},
				{0, 9},
				{0, 10},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dp, proportions := usecase.Min(tc.totalRoutes, tc.profitFunc)

			require.Equal(t, tc.expectedDP, dp, "DP table doesn't match expected")
			require.Equal(t, tc.expectedProportions, proportions, "Proportions table doesn't match expected")
		})
	}
}
func TestMax(t *testing.T) {
	testCases := []struct {
		name                string
		totalRoutes         int
		profitFunc          usecase.ProfitFunc
		expectedDP          [][]osmomath.Int
		expectedProportions [][]uint8
	}{
		{
			name:        "Single route",
			totalRoutes: 1,
			profitFunc: func(route int, proportion uint8) osmomath.Int {
				return osmomath.NewInt(int64(proportion))
			},
			expectedDP: [][]osmomath.Int{
				{zero, zero},
				{zero, osmomath.NewInt(1)},
				{zero, osmomath.NewInt(2)},
				{zero, osmomath.NewInt(3)},
				{zero, osmomath.NewInt(4)},
				{zero, osmomath.NewInt(5)},
				{zero, osmomath.NewInt(6)},
				{zero, osmomath.NewInt(7)},
				{zero, osmomath.NewInt(8)},
				{zero, osmomath.NewInt(9)},
				{zero, osmomath.NewInt(10)},
			},
			expectedProportions: [][]uint8{
				{0, 0},
				{0, 1},
				{0, 2},
				{0, 3},
				{0, 4},
				{0, 5},
				{0, 6},
				{0, 7},
				{0, 8},
				{0, 9},
				{0, 10},
			},
		},
		{
			name:        "Two routes with linear profit",
			totalRoutes: 2,
			profitFunc: func(route int, proportion uint8) osmomath.Int {
				return osmomath.NewInt(int64(proportion * uint8(route+1)))
			},
			expectedDP: [][]osmomath.Int{
				{zero, zero, zero},
				{zero, osmomath.NewInt(1), osmomath.NewInt(2)},
				{zero, osmomath.NewInt(2), osmomath.NewInt(4)},
				{zero, osmomath.NewInt(3), osmomath.NewInt(6)},
				{zero, osmomath.NewInt(4), osmomath.NewInt(8)},
				{zero, osmomath.NewInt(5), osmomath.NewInt(10)},
				{zero, osmomath.NewInt(6), osmomath.NewInt(12)},
				{zero, osmomath.NewInt(7), osmomath.NewInt(14)},
				{zero, osmomath.NewInt(8), osmomath.NewInt(16)},
				{zero, osmomath.NewInt(9), osmomath.NewInt(18)},
				{zero, osmomath.NewInt(10), osmomath.NewInt(20)},
			},
			expectedProportions: [][]uint8{
				{0, 0, 0},
				{0, 1, 1},
				{0, 2, 2},
				{0, 3, 3},
				{0, 4, 4},
				{0, 5, 5},
				{0, 6, 6},
				{0, 7, 7},
				{0, 8, 8},
				{0, 9, 9},
				{0, 10, 10},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dp, proportions := usecase.Max(tc.totalRoutes, tc.profitFunc)

			optimal := usecase.MaxBacktrack(tc.totalRoutes, proportions, tc.profitFunc)
			require.Equal(t, tc.expectedDP, dp, "DP table doesn't match expected", optimal)
			require.Equal(t, tc.expectedProportions, proportions, "Proportions table doesn't match expected")
		})
	}
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
