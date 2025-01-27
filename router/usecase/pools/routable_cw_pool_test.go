package pools_test

import (
	"context"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"

	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	"github.com/osmosis-labs/osmosis/v28/app/apptesting"

	"github.com/stretchr/testify/suite"
)

type CosmWasmPoolSuite struct {
	apptesting.KeeperTestHelper
}

func TestCosmWasmPoolSuite(t *testing.T) {
	suite.Run(t, new(CosmWasmPoolSuite))
}

func (s *CosmWasmPoolSuite) SetupTest() {
	s.Setup()
}

func (s *CosmWasmPoolSuite) newPool(method domain.TokenSwapMethod, coin sdk.Coin, denom string, isInvalidPoolType bool, takerFee osmomath.Dec, err error) domain.RoutablePool {
	cosmwasmPool := s.PrepareCustomTransmuterPoolCustomProject(s.TestAccs[0], []string{coin.Denom, denom}, "sqs", "scripts")

	mock := &mocks.MockRoutablePool{ChainPoolModel: cosmwasmPool.AsSerializablePool(), PoolType: poolmanagertypes.CosmWasm}
	wasmclient := &mocks.WasmClient{}

	token := "token_out"
	if method == domain.TokenSwapMethodExactOut {
		token = "token_in"
	}
	wasmclient.WithSmartContractState(
		[]byte(fmt.Sprintf(`{ "%s": { "denom" : "%s", "amount" : "%s" } }`, token, ETH, coin.Amount.String())),
		err,
	)

	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		Config: domain.CosmWasmPoolRouterConfig{
			GeneralCosmWasmCodeIDs: map[uint64]struct{}{
				cosmwasmPool.GetCodeId(): {},
			},
		},
		WasmClient:            wasmclient,
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}

	routablePool, err := pools.NewRoutablePool(mock, coin.Denom, denom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)

	// Overwrite pool type for edge case testing
	if isInvalidPoolType {
		mock.PoolType = poolmanagertypes.Concentrated
	}

	return routablePool
}

func (s *CosmWasmPoolSuite) TestCalculateTokenOutByTokenIn() {
	defaultAmount := DefaultAmt0
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount))

	tests := map[string]struct {
		tokenIn           sdk.Coin
		tokenOutDenom     string
		balances          sdk.Coins
		isInvalidPoolType bool
		expectError       error
	}{
		"valid CosmWasm quote": {
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

			routablePool := s.newPool(domain.TokenSwapMethodExactIn, tc.tokenIn, tc.tokenOutDenom, tc.isInvalidPoolType, noTakerFee, tc.expectError)

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

func (s *CosmWasmPoolSuite) TestChargeTakerFeeExactIn() {
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

			routablePool := s.newPool(domain.TokenSwapMethodExactIn, tc.tokenIn, "", false, tc.takerFee, nil)

			tokenAfterFee := routablePool.ChargeTakerFeeExactIn(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}

func (s *CosmWasmPoolSuite) TestCalculateTokenInByTokenOut() {
	defaultAmount := DefaultAmt0
	defaultBalances := sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount), sdk.NewCoin(ETH, defaultAmount))

	tests := map[string]struct {
		tokenOut          sdk.Coin
		tokenInDenom      string
		balances          sdk.Coins
		isInvalidPoolType bool
		expectError       error
	}{
		"valid CosmWasm quote": {
			tokenOut:     sdk.NewCoin(USDC, defaultAmount),
			tokenInDenom: ETH,
			balances:     defaultBalances,
		},
		"no error: token in is larger than balance of token in": {
			tokenOut:     sdk.NewCoin(USDC, defaultAmount),
			tokenInDenom: ETH,
			// Make token in amount 1 smaller than the default amount
			balances: sdk.NewCoins(sdk.NewCoin(USDC, defaultAmount.Sub(osmomath.OneInt())), sdk.NewCoin(ETH, defaultAmount)),
		},
		"error: token in is larger than balance of token out": {
			tokenOut:     sdk.NewCoin(USDC, defaultAmount),
			tokenInDenom: ETH,

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

			routablePool := s.newPool(domain.TokenSwapMethodExactOut, tc.tokenOut, tc.tokenInDenom, tc.isInvalidPoolType, noTakerFee, tc.expectError)

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

func (s *CosmWasmPoolSuite) TestChargeTakerFeeExactOut() {
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

			routablePool := s.newPool(domain.TokenSwapMethodExactOut, tc.tokenIn, "", false, tc.takerFee, nil)

			tokenAfterFee := routablePool.ChargeTakerFeeExactOut(tc.tokenIn)

			s.Require().Equal(tc.expectedToken, tokenAfterFee)
		})
	}
}
