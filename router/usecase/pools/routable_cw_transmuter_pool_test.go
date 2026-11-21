package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
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
		"no error: token out is larger than balance of token in": {
			tokenOut:     sdk.NewCoin(ETH, defaultAmount),
			tokenInDenom: USDC,
			// Make token in amount 1 smaller than the default amount
			balances: sdk.NewCoins(sdk.NewCoin(ETH, defaultAmount), sdk.NewCoin(USDC, defaultAmount.Sub(osmomath.OneInt()))),
		},
		"error: token out is larger than balance of token out": {
			tokenOut:     sdk.NewCoin(ETH, defaultAmount),
			tokenInDenom: USDC,

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
