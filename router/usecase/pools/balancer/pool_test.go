package balancer_test

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
	"github.com/osmosis-labs/osmosis/v31/app/apptesting"
	cosmwasmpooltypes "github.com/osmosis-labs/osmosis/v31/x/cosmwasmpool/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
)

type PoolTestSuite struct {
	apptesting.ConcentratedKeeperTestHelper
}

func TestRoutablePoolTestSuite(t *testing.T) {
	suite.Run(t, new(PoolTestSuite))
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

func (s *PoolTestSuite) PrepareCustomTransmuterPool(owner sdk.AccAddress, denoms []string) cosmwasmpooltypes.CosmWasmExtension {
	return s.PrepareCustomTransmuterPoolCustomProject(owner, denoms, "sqs", "scripts")
}

// Test quote logic over a specific pool that is of CFMM type.
// CFMM pools are balancer and stableswap.
func (s *PoolTestSuite) TestCalculateTokenOutByTokenIn_CFMM() {
	tests := map[string]struct {
		tokenIn          sdk.Coin
		tokenOutDenom    string
		poolType         poolmanagertypes.PoolType
		expectedTokenOut sdk.Coin
		expectError      error
	}{
		"balancer pool - valid calculation": {
			tokenIn:       sdk.NewCoin(Denom0, osmomath.NewInt(100)),
			tokenOutDenom: Denom0,
			poolType:      poolmanagertypes.Balancer,
		},
		"balancer pool - amount out bigger than int64": {
			tokenIn:       sdk.NewCoin(Denom1, osmomath.MustNewDecFromStr("10000000000000000000").TruncateInt()),
			tokenOutDenom: Denom1,
			poolType:      poolmanagertypes.Balancer,
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			balancerPoolID := s.PrepareBalancerPoolWithCoins(sdk.NewCoins(
				sdk.NewCoin(Denom0, osmomath.NewInt(1000000000000000000)),
				sdk.NewCoin(Denom1, osmomath.MustNewDecFromStr("100000000000000000000000").TruncateInt()),
			)...)

			pool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, balancerPoolID)
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
func (s *PoolTestSuite) TestCalculateTokenInByTokenOut_CFMM() {
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

func BenchmarkCalculateTokenOutByTokenIn(b *testing.B) {
	s := apptesting.ConcentratedKeeperTestHelper{}
	s.SetT(&testing.T{})
	s.Setup()

	tc := struct {
		tokenIn          sdk.Coin
		tokenOutDenom    string
		poolType         poolmanagertypes.PoolType
		expectedTokenOut sdk.Coin
		expectError      error
	}{
		tokenIn:       sdk.NewCoin(Denom0, osmomath.NewInt(100)),
		tokenOutDenom: Denom0,
		poolType:      poolmanagertypes.Balancer,
	}

	balancerPoolID := s.PrepareBalancerPoolWithCoins(sdk.NewCoins(
		sdk.NewCoin(Denom0, osmomath.NewInt(1000000000000000000)),
		sdk.NewCoin(Denom1, osmomath.MustNewDecFromStr("100000000000000000000000").TruncateInt()),
	)...)

	pool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, balancerPoolID)
	s.Require().NoError(err)

	mock := &mocks.MockRoutablePool{ChainPoolModel: pool, PoolType: tc.poolType}
	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}

	routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, tc.tokenOutDenom, noTakerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)

	for i := 0; i < b.N; i++ {
		routablePool.CalculateTokenOutByTokenIn(context.TODO(), tc.tokenIn)
	}
}
