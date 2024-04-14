package chainpricing_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
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

	defaultPricingRouterConfig = routertesting.DefaultPricingRouterConfig
	defaultPricingConfig       = routertesting.DefaultPricingConfig
)

func (s *PricingTestSuite) TestGetPrices_Chain() {

	// Set up mainnet mock state.
	router, mainnetState := s.SetupMainnetRouter(defaultPricingRouterConfig, defaultPricingConfig)
	mainnetUsecase := s.SetupRouterAndPoolsUsecase(router, mainnetState)

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
