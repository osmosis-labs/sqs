package pools_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
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
	defaultTickModel = &ingesttypes.TickModel{
		Ticks:            []ingesttypes.LiquidityDepthsWithRange{},
		CurrentTickIndex: 0,
		HasNoLiquidity:   false,
	}

	noTakerFee = osmomath.ZeroDec()
)

func (s *RoutablePoolTestSuite) PrepareCustomTransmuterPool(owner sdk.AccAddress, denoms []string) cosmwasmpooltypes.CosmWasmExtension {
	return s.PrepareCustomTransmuterPoolCustomProject(owner, denoms, "sqs", "scripts")
}

// Test quote logic over a specific pool that is of CFMM type.
// CFMM pools are balancer and stableswap.
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
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, tc.tokenOutDenom, noTakerFee, cosmWasmPoolsParams)
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

// Test quote logic over a specific pool that is of CFMM type.
// CFMM pools are balancer and stableswap.
func (s *RoutablePoolTestSuite) TestCalculateTokenInByTokenOut_CFMM() {
	tests := map[string]struct {
		tokenOut         sdk.Coin
		tokenInDenom     string
		poolType         poolmanagertypes.PoolType
		expectedTokenOut sdk.Coin
		expectError      error
	}{
		"balancer pool - valid calculation": {
			tokenOut:     sdk.NewCoin("foo", osmomath.NewInt(100)),
			tokenInDenom: "bar",
			poolType:     poolmanagertypes.Balancer,
		},
		"stableswap pool - valid calculation": {
			tokenOut:     sdk.NewCoin("foo", osmomath.NewInt(100)),
			tokenInDenom: "bar",
			poolType:     poolmanagertypes.Stableswap,
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
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenInDenom, tc.tokenOut.Denom, noTakerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)

			tokenOut, err := routablePool.CalculateTokenInByTokenOut(context.TODO(), tc.tokenOut)

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

func (s *RoutablePoolTestSuite) TestChargeTakerFeeExactIn_CCFM() {
	tests := map[string]struct {
		poolType      poolmanagertypes.PoolType
		tokenIn       sdk.Coin
		takerFee      osmomath.Dec
		expectedToken sdk.Coin
	}{
		"balancer pool - no taker fee": {
			poolType:      poolmanagertypes.Balancer,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"stableswap pool - no taker fee": {
			poolType:      poolmanagertypes.Stableswap,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"balancer pool - small taker fee": {
			poolType:      poolmanagertypes.Balancer,
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),          // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(99)), // 100 - 1 = 99
		},
		"stableswap pool - small taker fee": {
			poolType:      poolmanagertypes.Stableswap,
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),          // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(99)), // 100 - 1 = 99
		},
		"balancer pool - large taker fee": {
			poolType:      poolmanagertypes.Balancer,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),          // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(50)), // 100 - 50 = 50
		},
		"stableswap pool - large taker fee": {
			poolType:      poolmanagertypes.Stableswap,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),          // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(50)), // 100 - 50 = 50
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

			routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, "", tc.takerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)

			tokenAfterFee := routablePool.ChargeTakerFeeExactIn(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}

func (s *RoutablePoolTestSuite) TestChargeTakerFeeExactOut_CCFM() {
	tests := map[string]struct {
		poolType      poolmanagertypes.PoolType
		tokenIn       sdk.Coin
		takerFee      osmomath.Dec
		expectedToken sdk.Coin
	}{
		"balancer pool - no taker fee": {
			poolType:      poolmanagertypes.Balancer,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"stableswap pool - no taker fee": {
			poolType:      poolmanagertypes.Stableswap,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"balancer pool - small taker fee": {
			poolType:      poolmanagertypes.Balancer,
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),           // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(102)), // 100 + 1 = 101.01  = 102 (round up)
		},
		"stableswap pool - small taker fee": {
			poolType:      poolmanagertypes.Stableswap,
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),           // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(102)), // 100 + 1 = 101.01  = 102 (round up)
		},
		"balancer pool - large taker fee": {
			poolType:      poolmanagertypes.Balancer,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),           // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(200)), // 100 + 100 = 200
		},
		"stableswap pool - large taker fee": {
			poolType:      poolmanagertypes.Stableswap,
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),           // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(200)), // 100 + 100 = 200
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

			routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, "", tc.takerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)

			tokenAfterFee := routablePool.ChargeTakerFeeExactOut(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}
