package pools_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/stretchr/testify/suite"

	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/app/apptesting"
	cosmwasmpooltypes "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

type RoutablePoolTestSuite struct {
	apptesting.ConcentratedKeeperTestHelper
}

func TestRoutablePoolTestSuite(t *testing.T) {
	suite.Run(t, new(RoutablePoolTestSuite))
}

const (
	defaultPoolID = uint64(1)
)

var (
	// Concentrated liquidity constants
	ETH    = apptesting.ETH
	USDC   = apptesting.USDC
	USDT   = "usdt"
	ALLUSD = "allusd"
	Denom0 = ETH
	Denom1 = USDC

	DefaultCurrentTick = apptesting.DefaultCurrTick

	DefaultAmt0 = apptesting.DefaultAmt0
	DefaultAmt1 = apptesting.DefaultAmt1

	DefaultCoin0 = apptesting.DefaultCoin0
	DefaultCoin1 = apptesting.DefaultCoin1

	DefaultLiquidityAmt = apptesting.DefaultLiquidityAmt

	// router specific variables
	defaultTickModel = &sqsdomain.TickModel{
		Ticks:            []sqsdomain.LiquidityDepthsWithRange{},
		CurrentTickIndex: 0,
		HasNoLiquidity:   false,
	}

	noTakerFee = osmomath.ZeroDec()
)

func (s *RoutablePoolTestSuite) PrepareCustomTransmuterPool(owner sdk.AccAddress, denoms []string) cosmwasmpooltypes.CosmWasmExtension {
	return s.PrepareCustomTransmuterPoolCustomProject(owner, denoms, "sqs", "scripts")
}

// Test quote logic over a specific pool that is of CFMM type.
// CFMM pools are balancert and stableswap.
func (s *RoutablePoolTestSuite) TestCalculateTokenOutByTokenIn_CFMM() {
	tests := map[string]struct {
		tokenIn          sdk.Coin
		tokenOutDenom    string
		poolType         poolmanagertypes.PoolType
		expectedTokenOut sdk.Coin
		expectError      error
	}{
		"balancer pool - valid calculation": {
			tokenIn:       sdk.NewCoin("foo", osmomath.NewInt(100)),
			tokenOutDenom: "bar",
			poolType:      poolmanagertypes.Balancer,
		},
		"stableswap pool - valid calculation": {
			tokenIn:       sdk.NewCoin("foo", osmomath.NewInt(100)),
			tokenOutDenom: "bar",
			poolType:      poolmanagertypes.Stableswap,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			poolID := s.CreatePoolFromType(tc.poolType)
			pool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, poolID)
			s.Require().NoError(err)

			mock := &mocks.MockRoutablePool{ChainPoolModel: pool, PoolType: tc.poolType}
			cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
				ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
			}
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenOutDenom, noTakerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)

			tokenOut, err := routablePool.CalculateTokenOutByTokenIn(context.TODO(), tc.tokenIn)

			if tc.expectError != nil {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)

			// We don't check the exact amount because the correctness of calculations is tested
			// at the pool model layer of abstraction. Here, the goal is to make sure that we get
			// a positive amount when the pool is valid.
			s.Require().True(tokenOut.IsPositive())
		})
	}
}
