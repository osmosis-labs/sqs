package routertesting

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/v22/app/apptesting"
	"github.com/osmosis-labs/osmosis/v22/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v22/x/poolmanager/types"
)

type RouterTestHelper struct {
	apptesting.ConcentratedKeeperTestHelper
}

const DefaultPoolID = uint64(1)

var (
	// Concentrated liquidity constants
	ETH    = apptesting.ETH
	USDC   = apptesting.USDC
	USDT   = "usdt"
	Denom0 = ETH
	Denom1 = USDC

	DefaultCurrentTick = apptesting.DefaultCurrTick

	DefaultAmt0 = apptesting.DefaultAmt0
	DefaultAmt1 = apptesting.DefaultAmt1

	DefaultCoin0 = apptesting.DefaultCoin0
	DefaultCoin1 = apptesting.DefaultCoin1

	DefaultLiquidityAmt = apptesting.DefaultLiquidityAmt

	// router specific variables
	DefaultTickModel = &sqsdomain.TickModel{
		Ticks:            []sqsdomain.LiquidityDepthsWithRange{},
		CurrentTickIndex: 0,
		HasNoLiquidity:   false,
	}

	NoTakerFee = osmomath.ZeroDec()

	DefaultTakerFee     = osmomath.MustNewDecFromStr("0.002")
	DefaultPoolBalances = sdk.NewCoins(
		sdk.NewCoin(DenomOne, DefaultAmt0),
		sdk.NewCoin(DenomTwo, DefaultAmt1),
	)
	DefaultSpreadFactor = osmomath.MustNewDecFromStr("0.005")

	DefaultPool = &mocks.MockRoutablePool{
		ID:                   DefaultPoolID,
		Denoms:               []string{DenomOne, DenomTwo},
		TotalValueLockedUSDC: osmomath.NewInt(10),
		PoolType:             poolmanagertypes.Balancer,
		Balances:             DefaultPoolBalances,
		TakerFee:             DefaultTakerFee,
		SpreadFactor:         DefaultSpreadFactor,
	}
	EmptyRoute = route.RouteImpl{}

	// Test denoms
	DenomOne   = denomNum(1)
	DenomTwo   = denomNum(2)
	DenomThree = denomNum(3)
	DenomFour  = denomNum(4)
	DenomFive  = denomNum(5)
	DenomSix   = denomNum(6)
)

func denomNum(i int) string {
	return fmt.Sprintf("denom%d", i)
}

// Note that it does not deep copy pools
func WithRoutePools(r route.RouteImpl, pools []sqsdomain.RoutablePool) route.RouteImpl {
	newRoute := route.RouteImpl{
		Pools: make([]sqsdomain.RoutablePool, 0, len(pools)),
	}

	newRoute.Pools = append(newRoute.Pools, pools...)

	return newRoute
}

// Note that it does not deep copy pools
func WithCandidateRoutePools(r sqsdomain.CandidateRoute, pools []sqsdomain.CandidatePool) sqsdomain.CandidateRoute {
	newRoute := sqsdomain.CandidateRoute{
		Pools: make([]sqsdomain.CandidatePool, 0, len(pools)),
	}

	newRoute.Pools = append(newRoute.Pools, pools...)
	return newRoute
}

// ValidateRoutePools validates that the expected pools are equal to the actual pools.
// Specifically, validates the following fields:
// - ID
// - Type
// - Balances
// - Spread Factor
// - Token Out Denom
// - Taker Fee
func (s *RouterTestHelper) ValidateRoutePools(expectedPools []sqsdomain.RoutablePool, actualPools []sqsdomain.RoutablePool) {
	s.Require().Equal(len(expectedPools), len(actualPools))

	for i, expectedPool := range expectedPools {
		actualPool := actualPools[i]

		expectedResultPool, ok := expectedPool.(domain.RoutableResultPool)
		s.Require().True(ok)

		// Cast to result pool
		actualResultPool, ok := actualPool.(domain.RoutableResultPool)
		s.Require().True(ok)

		s.Require().Equal(expectedResultPool.GetId(), actualResultPool.GetId())
		s.Require().Equal(expectedResultPool.GetType(), actualResultPool.GetType())
		s.Require().Equal(expectedResultPool.GetBalances().String(), actualResultPool.GetBalances().String())
		s.Require().Equal(expectedResultPool.GetSpreadFactor().String(), actualResultPool.GetSpreadFactor().String())
		s.Require().Equal(expectedResultPool.GetTokenOutDenom(), actualResultPool.GetTokenOutDenom())
		s.Require().Equal(expectedResultPool.GetTakerFee().String(), actualResultPool.GetTakerFee().String())
	}
}
