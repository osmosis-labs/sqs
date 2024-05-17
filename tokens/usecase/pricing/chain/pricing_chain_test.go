package chainpricing_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/osmoutils/osmoassert"
	"github.com/osmosis-labs/sqs/domain"
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
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	s.Require().NotZero(len(routertesting.MainnetDenoms))
	for _, mainnetDenom := range routertesting.MainnetDenoms {
		mainnetDenom := mainnetDenom
		s.Run(mainnetDenom, func() {

			// System under test.
			usdcPrice, err := pricingStrategy.GetPrice(context.Background(), mainnetDenom, USDC)
			s.Require().NoError(err)

			usdtPrice, err := pricingStrategy.GetPrice(context.Background(), mainnetDenom, USDT)
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
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(mainnetState, routertesting.WithRouterConfig(defaultPricingRouterConfig), routertesting.WithPricingConfig(defaultPricingConfig))

	// Set up on-chain pricing strategy
	pricingStrategy, err := pricing.NewPricingStrategy(defaultPricingConfig, mainnetUsecase.Tokens, mainnetUsecase.Router)
	s.Require().NoError(err)

	priceQuoteBasedMethod, err := pricingStrategy.GetPrice(context.Background(), DYDX, USDC, domain.WithRecomputePricesQuoteBasedMethod())
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
	priceSpotPriceMethod, err := pricingStrategy.GetPrice(context.Background(), DYDX, USDC, domain.WithRecomputePrices())
	s.Require().NoError(err)
	s.Require().NotZero(priceSpotPriceMethod)

	// 0.1 additive tolerance.
	osmoassert.DecApproxEq(s.T(), priceQuoteBasedMethod.Dec(), priceSpotPriceMethod.Dec(), osmomath.MustNewDecFromStr("0.1"))
}
