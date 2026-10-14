package chainpricing_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/osmoutils/osmoassert"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing"
	"github.com/stretchr/testify/suite"
)

func TestPricingTestSuite(t *testing.T) {
	suite.Run(t, new(PricingTestSuite))
}

type PricingTestSuite struct {
	routertesting.RouterTestHelper
}

var (
	UOSMO   = routertesting.UOSMO
	ATOM    = routertesting.ATOM
	stOSMO  = routertesting.STOSMO
	stATOM  = routertesting.STATOM
	USDC    = routertesting.USDC
	USDCaxl = routertesting.USDCaxl
	USDT    = routertesting.USDT
	WBTC    = routertesting.WBTC
	ETH     = routertesting.ETH
	AKT     = routertesting.AKT
	UMEE    = routertesting.UMEE
	UION    = routertesting.UION
	CRE     = routertesting.CRE
	DYDX    = routertesting.DYDX

	defaultPricingRouterConfig = routertesting.DefaultPricingRouterConfig
	defaultPricingConfig       = routertesting.DefaultPricingConfig
)

func (s *PricingTestSuite) TestGetPrices_Chain() {
	// Set up mainnet mock state.
	mainnetState := s.SetupMainnetState()

	// Run the test for the mainnet state.
	s.testGetPrices_Chain(mainnetState)
}

func (s *PricingTestSuite) TestGetPrices_Chain_BE_758() {
	// Set up mainnet mock state.
	mainnetState := s.SetupMainnetStatePath("BE-758")

	// Run the test for the mainnet state.
	s.testGetPrices_Chain(mainnetState)
}

func (s *PricingTestSuite) testGetPrices_Chain(mainnetState routertesting.MockMainnetState) {
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	s.Require().NotZero(len(routertesting.MainnetDenoms))
	for _, mainnetDenom := range routertesting.MainnetDenoms {
		s.Run(mainnetDenom, func() {

			// System under test.
			usdcPrice, _, err := pricingStrategy.GetPrice(context.Background(), mainnetDenom, USDC)
			s.Require().NoError(err)

			usdtPrice, _, err := pricingStrategy.GetPrice(context.Background(), mainnetDenom, USDT)
			s.Require().NoError(err)

			errTolerance := osmomath.ErrTolerance{
				// 1% tolerance
				MultiplicativeTolerance: osmomath.MustNewDecFromStr("0.07"),
			}

			result := errTolerance.CompareBigDec(usdcPrice, usdtPrice)
			s.Require().Zero(result, fmt.Sprintf("denom: %s, usdcPrice: %s, usdtPrice: %s", mainnetDenom, usdcPrice, usdtPrice))
		})
	}
}

// This test validates that the pricing strategy can compute the price of a token pair
// using both the quote based and the spot price based methods.
//
// It compares the results and ensures that the difference is within a reasonable range.
func (s *PricingTestSuite) TestComputePrice_QuoteBasedMethod() {
	// Set up mainnet mock state.
	mainnetState := s.SetupMainnetState()

	// Run the test for the mainnet state.
	s.testComputePrice_QuoteBasedMethod(mainnetState)
}

// This test validates that the pricing strategy can compute the price of a token pair
// using both the quote based and the spot price based methods.
//
// It compares the results and ensures that the difference is within a reasonable range.
func (s *PricingTestSuite) TestComputePrice_QuoteBasedMethod_BE_758() {
	// Set up mainnet mock state.
	mainnetState := s.SetupMainnetStatePath("BE-758")

	// Run the test for the mainnet state.
	s.testComputePrice_QuoteBasedMethod(mainnetState)
}

func (s *PricingTestSuite) testComputePrice_QuoteBasedMethod(mainnetState routertesting.MockMainnetState) {
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	priceQuoteBasedMethod, _, err := pricingStrategy.GetPrice(context.Background(), DYDX, USDC, domain.WithRecomputePricesQuoteBasedMethod())
	s.Require().NoError(err)
	s.Require().NotZero(priceQuoteBasedMethod)

	// Ensure the price is within a reasonable range.
	// This test specifically aims to catch incorrect computation of prices by accidentally scaling them up
	// When using the quote method.
	// Since DYDX has precision of 18 and USDC has precision of 6, the price should be less than 10^12 to be considered reasonable.
	// to be considered reasonable.
	// We, however, make the assertion stricter by requiring the price to be less than 10^6.
	s.Require().True(priceQuoteBasedMethod.LT(osmomath.NewBigDec(1_000_000)))

	// Recompute using spot-price method
	priceSpotPriceMethod, _, err := pricingStrategy.GetPrice(context.Background(), DYDX, USDC, domain.WithRecomputePrices())
	s.Require().NoError(err)
	s.Require().NotZero(priceSpotPriceMethod)

	// 0.1 additive tolerance.
	osmoassert.DecApproxEq(s.T(), priceQuoteBasedMethod.Dec(), priceSpotPriceMethod.Dec(), osmomath.MustNewDecFromStr("0.1"))
}

// This test validates that chain pricing never prices through a pool listed in the
// router config's excluded pool IDs, and still finds a price through other pools.
func (s *PricingTestSuite) TestGetPrice_ExcludedPoolIDs() {
	mainnetState := s.SetupMainnetState()

	poolIDsInRoutes := func(routes []domain.SplitRoute) map[uint64]struct{} {
		poolIDs := map[uint64]struct{}{}
		for _, route := range routes {
			for _, pool := range route.GetPools() {
				poolIDs[pool.GetId()] = struct{}{}
			}
		}
		return poolIDs
	}

	// Find a pool that the default config prices OSMO through.
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	_, routes, err := pricingStrategy.GetPrice(context.Background(), UOSMO, USDC)
	s.Require().NoError(err)
	s.Require().NotEmpty(routes)
	excludedPoolID := routes[0].GetPools()[0].GetId()

	// Exclude that pool and price again.
	routerConfig := defaultPricingRouterConfig
	routerConfig.ExcludedPoolIDs = []uint64{excludedPoolID}

	mainnetUsecase = s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(routerConfig), routertesting.WithPricingConfig(defaultPricingConfig))
	pricingStrategy, err = pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	// System under test.
	price, routes, err := pricingStrategy.GetPrice(context.Background(), UOSMO, USDC)
	s.Require().NoError(err)
	s.Require().True(price.IsPositive())
	s.Require().NotEmpty(routes)
	s.Require().NotContains(poolIDsInRoutes(routes), excludedPoolID)
}

// Validates that a failed recompute evicts the cached price, so that the last good price
// is not served after the route stops being usable.
func (s *PricingTestSuite) TestGetPrice_FailedRecomputeEvictsCachedPrice() {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	routeUnusable := false
	spotPricePanics := false
	routerMock := &mocks.RouterUsecaseMock{
		GetSimpleQuoteFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, []domain.SplitRoute, error) {
			if routeUnusable {
				return nil, nil, errors.New("no route")
			}
			return mainnetUsecase.Router.GetSimpleQuote(ctx, tokenIn, tokenOutDenom, opts...)
		},
		GetPoolSpotPriceFunc: func(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
			if spotPricePanics {
				panic("malformed spot price")
			}
			return mainnetUsecase.Router.GetPoolSpotPrice(ctx, poolID, quoteAsset, baseAsset)
		},
	}

	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, routerMock)
	s.Require().NoError(err)

	// Populate the cache.
	price, _, err := pricingStrategy.GetPrice(context.Background(), ATOM, USDC, domain.WithRecomputePrices())
	s.Require().NoError(err)
	s.Require().True(price.IsPositive())

	routeUnusable = true

	// Still served from cache.
	cachedPrice, _, err := pricingStrategy.GetPrice(context.Background(), ATOM, USDC)
	s.Require().NoError(err)
	s.Require().Equal(price, cachedPrice)

	// System under test: the failed recompute evicts the cached price.
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC, domain.WithRecomputePrices())
	s.Require().Error(err)

	// The old price is no longer served.
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC)
	s.Require().Error(err)

	// A panicking recompute evicts as well and is returned as an error.
	routeUnusable = false
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC, domain.WithRecomputePrices())
	s.Require().NoError(err)

	spotPricePanics = true
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC, domain.WithRecomputePrices())
	s.Require().Error(err)

	routeUnusable = true
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC)
	s.Require().Error(err)
}

// Validates that a failed price computation is served from the failed price cache, so that a pair without
// a usable route is not searched again on every request, while recomputes still search and a success clears it.
// A failure caused by the request's context is not cached.
func (s *PricingTestSuite) TestGetPrice_FailedPriceIsCached() {
	mainnetState := s.SetupMainnetState()
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	routeUnusable := true
	quoteCalls := 0
	routerMock := &mocks.RouterUsecaseMock{
		GetSimpleQuoteFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, []domain.SplitRoute, error) {
			quoteCalls++
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if routeUnusable {
				return nil, nil, errors.New("no route")
			}
			return mainnetUsecase.Router.GetSimpleQuote(ctx, tokenIn, tokenOutDenom, opts...)
		},
		GetPoolSpotPriceFunc: func(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
			return mainnetUsecase.Router.GetPoolSpotPrice(ctx, poolID, quoteAsset, baseAsset)
		},
	}

	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, routerMock)
	s.Require().NoError(err)

	// A cancelled request fails without caching the failure.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = pricingStrategy.GetPrice(cancelledCtx, ATOM, USDC)
	s.Require().Error(err)
	s.Require().Equal(1, quoteCalls)

	// The first failure searches; the second is served from the failed price cache.
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC)
	s.Require().Error(err)
	s.Require().Equal(2, quoteCalls)

	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC)
	s.Require().Error(err)
	s.Require().Equal(2, quoteCalls)

	// Like a cached price, the failure also answers a search at another min pool liquidity cap or in the
	// other direction, so the pricing worker's failures spare requests the search.
	_, _, err = pricingStrategy.GetPrice(context.Background(), ATOM, USDC, domain.WithMinPricingPoolLiquidityCap(123_456_789))
	s.Require().Error(err)
	s.Require().Equal(2, quoteCalls)

	_, _, err = pricingStrategy.GetPrice(context.Background(), USDC, ATOM)
	s.Require().Error(err)
	s.Require().Equal(2, quoteCalls)

	// A recompute ignores the failed price cache, and its success clears it.
	routeUnusable = false
	price, _, err := pricingStrategy.GetPrice(context.Background(), ATOM, USDC, domain.WithRecomputePrices())
	s.Require().NoError(err)
	s.Require().True(price.IsPositive())
	s.Require().Equal(3, quoteCalls)

	cachedPrice, _, err := pricingStrategy.GetPrice(context.Background(), ATOM, USDC)
	s.Require().NoError(err)
	s.Require().Equal(price, cachedPrice)
	s.Require().Equal(3, quoteCalls)
}
