package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

// Tests no slippage quotes and validation edge cases aroun transmuter pools.
func (s *RoutablePoolTestSuite) TestCalculateTokenOutByTokenIn_Transmuter() {
	defaultAmount := DefaultAmt0
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount))

	tests := map[string]struct {
		tokenIn           sdk.Coin
		tokenOutDenom     string
		balances          sdk.Coins
		isInvalidPoolType bool
		expectError       error
	}{
		"valid transmuter quote": {
			tokenIn:       sdk.NewCoin(USDC, defaultAmount),
			tokenOutDenom: ETH,
			balances:      defaultBalances,
		},
		"no error: token in is larger than balance of token in": {
			tokenIn:       sdk.NewCoin(USDC, defaultAmount),
			tokenOutDenom: ETH,
			// Make token in amount 1 smaller than the default amount
			balances: sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount.Sub(osmomath.OneInt())), sdk.NewCoin(ETH, defaultAmount)),
		},
		"error: token in is larger than balance of token out": {
			tokenIn:       sdk.NewCoin(USDC, defaultAmount),
			tokenOutDenom: ETH,

			// Make token out amount 1 smaller than the default amount
			balances: sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount.Sub(osmomath.OneInt()))),

			expectError: domain.TransmuterInsufficientBalanceError{
				Denom:         ETH,
				BalanceAmount: defaultAmount.Sub(osmomath.OneInt()).String(),
				Amount:        defaultAmount.String(),
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tc.tokenIn.Denom, tc.tokenOutDenom})

			poolType := cosmwasmPool.GetType()

			mock := &mocks.MockRoutablePool{ChainPoolModel: cosmwasmPool.AsSerializablePool(), Balances: tc.balances, PoolType: poolType}

			cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
				Config: domain.CosmWasmPoolRouterConfig{
					TransmuterCodeIDs: map[uint64]struct{}{
						cosmwasmPool.GetCodeId(): {},
					},
				},
				ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
			}
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, tc.tokenOutDenom, noTakerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)

			// Overwrite pool type for edge case testing
			if tc.isInvalidPoolType {
				mock.PoolType = poolmanagertypes.Concentrated
			}

			tokenOut, err := routablePool.CalculateTokenOutByTokenIn(context.TODO(), tc.tokenIn)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			// No slippage swaps on success
			s.Require().Equal(tc.tokenIn.Amount, tokenOut.Amount)
		})
	}
}

func (s *RoutablePoolTestSuite) TestChargeTakerFeeExactIn_Transmuter() {
	defaultAmount := DefaultAmt0
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount))

	tests := map[string]struct {
		poolType      poolmanagertypes.PoolType
		tokenIn       sdk.Coin
		takerFee      osmomath.Dec
		balances      sdk.Coins
		expectedToken sdk.Coin
	}{
		"no taker fee": {
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			balances:      defaultBalances,
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"small taker fee": {
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),          // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(99)), // 100 - 1 = 99
		},
		"large taker fee": {
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),          // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(50)), // 100 - 50 = 50
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{USDC, ETH})

			poolType := cosmwasmPool.GetType()

			mock := &mocks.MockRoutablePool{ChainPoolModel: cosmwasmPool.AsSerializablePool(), Balances: tc.balances, PoolType: poolType}

			cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
				Config: domain.CosmWasmPoolRouterConfig{
					TransmuterCodeIDs: map[uint64]struct{}{
						cosmwasmPool.GetCodeId(): {},
					},
				},
				ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
			}
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, "", tc.takerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)
			s.Require().NoError(err)

			tokenAfterFee := routablePool.ChargeTakerFeeExactIn(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}

func (s *RoutablePoolTestSuite) TestCalculateTokenInByTokenOut_Transmuter() {
	defaultAmount := DefaultAmt0
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount))

	tests := map[string]struct {
		tokenOut          sdk.Coin
		tokenInDenom      string
		balances          sdk.Coins
		isInvalidPoolType bool
		expectError       error
	}{
		"valid transmuter quote": {
			tokenOut:     sdk.NewCoin(ETH, defaultAmount),
			tokenInDenom: USDC,
			balances:     defaultBalances,
		},
		"no error: token out is larger than balance of token out": {
			tokenOut:     sdk.NewCoin(ETH, defaultAmount),
			tokenInDenom: USDC,
			// Make token out amount 1 smaller than the default amount
			balances: sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount.Sub(osmomath.OneInt()))),
		},
		"error: token out is larger than balance of token in": {
			tokenOut:     sdk.NewCoin(ETH, defaultAmount),
			tokenInDenom: USDC,

			// Make token in amount 1 smaller than the default amount
			balances: sdk.NewCoins(sdk.NewCoin(ETH, defaultAmount), sdk.NewCoin(USDC, defaultAmount.Sub(osmomath.OneInt()))),

			expectError: domain.TransmuterInsufficientBalanceError{
				Denom:         USDC,
				BalanceAmount: defaultAmount.Sub(osmomath.OneInt()).String(),
				Amount:        defaultAmount.String(),
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tc.tokenInDenom, tc.tokenOut.Denom})

			poolType := cosmwasmPool.GetType()

			mock := &mocks.MockRoutablePool{ChainPoolModel: cosmwasmPool.AsSerializablePool(), Balances: tc.balances, PoolType: poolType}

			cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
				Config: domain.CosmWasmPoolRouterConfig{
					TransmuterCodeIDs: map[uint64]struct{}{
						cosmwasmPool.GetCodeId(): {},
					},
				},
				ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
			}
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenInDenom, tc.tokenOut.Denom, noTakerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)

			// Overwrite pool type for edge case testing
			if tc.isInvalidPoolType {
				mock.PoolType = poolmanagertypes.Concentrated
			}

			tokenIn, err := routablePool.CalculateTokenInByTokenOut(context.TODO(), tc.tokenOut)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			// No slippage swaps on success
			s.Require().Equal(tc.tokenOut.Amount, tokenIn.Amount)
		})
	}
}

func (s *RoutablePoolTestSuite) TestChargeTakerFeeExactOut_Transmuter() {
	defaultAmount := DefaultAmt0
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount))

	tests := map[string]struct {
		poolType      poolmanagertypes.PoolType
		tokenIn       sdk.Coin
		takerFee      osmomath.Dec
		balances      sdk.Coins
		expectedToken sdk.Coin
	}{
		"no taker fee": {
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			balances:      defaultBalances,
			takerFee:      osmomath.NewDec(0),
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(100)),
		},
		"small taker fee": {
			tokenIn:       sdk.NewCoin(USDT, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(1, 2),           // 1%
			expectedToken: sdk.NewCoin(USDT, osmomath.NewInt(102)), // 100 + 1 = 101.01  = 102 (round up)
		},
		"large taker fee": {
			tokenIn:       sdk.NewCoin(USDC, osmomath.NewInt(100)),
			takerFee:      osmomath.NewDecWithPrec(5, 1),           // 50%
			expectedToken: sdk.NewCoin(USDC, osmomath.NewInt(200)), // 100 + 100 = 200
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()

			cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{USDC, ETH})

			poolType := cosmwasmPool.GetType()

			mock := &mocks.MockRoutablePool{ChainPoolModel: cosmwasmPool.AsSerializablePool(), Balances: tc.balances, PoolType: poolType}

			cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
				Config: domain.CosmWasmPoolRouterConfig{
					TransmuterCodeIDs: map[uint64]struct{}{
						cosmwasmPool.GetCodeId(): {},
					},
				},
				ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
			}
			routablePool, err := pools.NewRoutablePool(mock, tc.tokenIn.Denom, "", tc.takerFee, cosmWasmPoolsParams)
			s.Require().NoError(err)
			s.Require().NoError(err)

			tokenAfterFee := routablePool.ChargeTakerFeeExactOut(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}
