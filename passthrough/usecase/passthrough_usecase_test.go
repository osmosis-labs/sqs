package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/passthrough/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type PassthroughUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultAddress = "default-address"

	validGammSharePoolID = uint64(2)
)

var (
	USDC  = routertesting.USDC
	UOSMO = routertesting.UOSMO
	ATOM  = routertesting.ATOM
	WBTC  = routertesting.WBTC
	ETH   = routertesting.ETH

	grpcClientError       = errors.New("grpc-client-error")
	calcExitCFMMPoolError = errors.New("calc-exit-cfmm-pool-error")

	formatValidGammShare = func(poolID uint64) string {
		return fmt.Sprintf("%s/pool/%d", usecase.GammSharePrefix, poolID)
	}

	validGammShareDenom  = formatValidGammShare(validGammSharePoolID)
	validGammShareAmount = sdk.NewInt(3_000_000)

	nonShareDefaultBalances = sdk.NewCoins(
		sdk.NewCoin(UOSMO, sdk.NewInt(1_000_000)),
		sdk.NewCoin(ATOM, sdk.NewInt(2_000_000)),
	)

	defaultGammShareCoin = sdk.NewCoin(validGammShareDenom, validGammShareAmount)
	defaultBalances      = nonShareDefaultBalances.Add(defaultGammShareCoin)

	defaultExitPoolCoins = sdk.NewCoins(
		sdk.NewCoin(WBTC, sdk.NewInt(1_000_000)),
		sdk.NewCoin(ETH, sdk.NewInt(500_000)),
	)

	defaultConcentratedCoin = sdk.NewCoin(USDC, sdk.NewInt(1_000_000))

	defaultConcentratedShareCoin = sdk.NewCoin(usecase.ConcentratedSharePrefix+"/pool", sdk.NewInt(1_000_000))

	emptyCoins = sdk.Coins{}
)

func TestPassthroughUseCase(t *testing.T) {
	suite.Run(t, new(PassthroughUseCaseTestSuite))
}

// Tests the get locked coins method using mocks.
func (s *PassthroughUseCaseTestSuite) TestGetLockedCoins() {
	tests := []struct {
		name    string
		address string

		mockAccountLockedCoinsIfDefaultAddress sdk.Coins

		expectedCoins sdk.Coins
		expectedError error
	}{
		{
			name: "happy path",

			address: defaultAddress,

			mockAccountLockedCoinsIfDefaultAddress: defaultBalances,

			expectedCoins: nonShareDefaultBalances.Add(defaultExitPoolCoins...),
		},
		{
			name: "concentrated shares are skipped",

			address: defaultAddress,

			mockAccountLockedCoinsIfDefaultAddress: defaultBalances.Add(defaultConcentratedShareCoin),

			expectedCoins: nonShareDefaultBalances.Add(defaultExitPoolCoins...),
		},
		{
			name: "error: grpc client error",

			address: "wrong address",

			expectedError: grpcClientError,
		},
		{
			name: "skip error in converting gamm share to underlying coins",

			address: defaultAddress,

			mockAccountLockedCoinsIfDefaultAddress: nonShareDefaultBalances.Add(sdk.NewCoin(formatValidGammShare(validGammSharePoolID+1), validGammShareAmount)),

			// Note that only non share balances are returned
			// The share coins are skipped due to error.
			expectedCoins: nonShareDefaultBalances,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {

			// Initialize GRPC client mock
			grpcClientMock := mocks.PassthroughGRPCClientMock{
				MockAccountLockedCoinsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
					// If not default address, return grpc client error
					if address != defaultAddress {
						return sdk.Coins{}, grpcClientError
					}

					// If default address, return mock balances
					return tt.mockAccountLockedCoinsIfDefaultAddress, nil
				},
			}

			// Initialize pools use case mock
			poolsUseCaseMock := mocks.PoolsUsecaseMock{
				CalcExitCFMMPoolFunc: func(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error) {
					// If the pool ID is valid and the exiting shares are valid, return default exit pool coins
					if poolID == validGammSharePoolID && exitingShares.Equal(validGammShareAmount) {
						return defaultExitPoolCoins, nil
					}

					// Otherwise, return calcExitCFMMPoolError
					return sdk.Coins{}, calcExitCFMMPoolError
				},
			}

			pu := usecase.NewPassThroughUsecase(&grpcClientMock, &poolsUseCaseMock, nil, nil, nil, USDC, &log.NoOpLogger{})

			// System under test
			actualBalances, err := pu.GetLockedCoins(context.TODO(), tt.address)

			// Assert
			s.Require().Equal(tt.expectedCoins, actualBalances)
			s.Require().Equal(tt.expectedError, err)
		})
	}
}

// Tests the get all balances method using mocks.
func (s *PassthroughUseCaseTestSuite) TestGetAllBalances() {

	tests := []struct {
		name    string
		address string

		mockAllBalancesIfDefaultAddress sdk.Coins

		expectedCoins sdk.Coins
		expectedError error
	}{
		{
			name: "happy path",

			address: defaultAddress,

			mockAllBalancesIfDefaultAddress: defaultBalances,

			expectedCoins: nonShareDefaultBalances.Add(defaultExitPoolCoins...),
		},
		{
			name: "error: grpc client error",

			address: "wrong address",

			expectedError: grpcClientError,
		},
		{
			name: "skip error in converting gamm share to underlying coins",

			address: defaultAddress,

			mockAllBalancesIfDefaultAddress: nonShareDefaultBalances.Add(sdk.NewCoin(formatValidGammShare(validGammSharePoolID+1), validGammShareAmount)),

			// Note that only non share balances are returned
			// The share coins are skipped due to error.
			expectedCoins: nonShareDefaultBalances,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {

			// Initialize GRPC client mock
			grpcClientMock := mocks.PassthroughGRPCClientMock{
				MockAllBalancesCb: func(ctx context.Context, address string) (sdk.Coins, error) {
					// If not default address, return grpc client error
					if address != defaultAddress {
						return sdk.Coins{}, grpcClientError
					}

					// If default address, return mock balances
					return tt.mockAllBalancesIfDefaultAddress, nil
				},
			}

			// Initialize pools use case mock
			poolsUseCaseMock := mocks.PoolsUsecaseMock{
				CalcExitCFMMPoolFunc: func(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error) {
					// If the pool ID is valid and the exiting shares are valid, return default exit pool coins
					if poolID == validGammSharePoolID && exitingShares.Equal(validGammShareAmount) {
						return defaultExitPoolCoins, nil
					}

					// Otherwise, return calcExitCFMMPoolError
					return sdk.Coins{}, calcExitCFMMPoolError
				},
			}

			pu := usecase.NewPassThroughUsecase(&grpcClientMock, &poolsUseCaseMock, nil, nil, nil, USDC, &log.NoOpLogger{})

			// System under test
			actualBalances, err := pu.GetBankBalances(context.TODO(), tt.address)

			// Assert
			s.Require().Equal(tt.expectedCoins, actualBalances)
			s.Require().Equal(tt.expectedError, err)
		})
	}
}

// Test the handle gamm shares method using mocks.
func (s *PassthroughUseCaseTestSuite) TestHandleGammShares() {
	tests := []struct {
		name    string
		address string

		coinIn sdk.Coin

		expectedCoins sdk.Coins
		expectedError bool
	}{
		{
			name: "happy path",

			address: defaultAddress,

			coinIn: defaultGammShareCoin,

			expectedCoins: defaultExitPoolCoins,
		},
		{
			name: "error: non-gamm share coin",

			address: defaultAddress,

			coinIn: defaultConcentratedCoin,

			expectedError: true,
		},
		{
			name: "error: grpc client error",

			address: "wrong address",

			expectedError: true,
		},
		{
			name: "error: in converting gamm share to underlying coins",

			address: defaultAddress,

			coinIn: sdk.NewCoin(formatValidGammShare(validGammSharePoolID+1), validGammShareAmount),

			expectedError: true,
		},

		{
			name: "error: in parsing uint",

			address: defaultAddress,

			coinIn: sdk.NewCoin(fmt.Sprintf("%s/pool/%s", usecase.GammSharePrefix, "notuint"), validGammShareAmount),

			expectedError: true,
		},
		{
			name: "error: no forward slash in denom",

			address: defaultAddress,

			coinIn: sdk.NewCoin(usecase.GammSharePrefix, validGammShareAmount),

			expectedError: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {

			// Initialize pools use case mock
			poolsUseCaseMock := mocks.PoolsUsecaseMock{
				CalcExitCFMMPoolFunc: func(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error) {
					// If the pool ID is valid and the exiting shares are valid, return default exit pool coins
					if poolID == validGammSharePoolID && exitingShares.Equal(validGammShareAmount) {
						return defaultExitPoolCoins, nil
					}

					// Otherwise, return calcExitCFMMPoolError
					return sdk.Coins{}, calcExitCFMMPoolError
				},
			}

			pu := usecase.NewPassThroughUsecase(nil, &poolsUseCaseMock, nil, nil, nil, USDC, &log.NoOpLogger{})

			// System under test
			actualBalances, err := pu.HandleGammShares(tt.coinIn)

			// Assert
			if tt.expectedError {
				s.Require().Error(err)
				return
			}

			s.Require().NoError(err)
			s.Require().Equal(tt.expectedCoins, actualBalances)
		})
	}
}
