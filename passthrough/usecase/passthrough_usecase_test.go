package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
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
	miscError             = errors.New("misc-error")

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

	zero = osmomath.ZeroDec()
)

func TestPassthroughUseCase(t *testing.T) {
	suite.Run(t, new(PassthroughUseCaseTestSuite))
}

// Tests the compute capitalization for coins method using mocks.
func (s *PassthroughUseCaseTestSuite) TestComputeCapitalizationForCoins() {
	var (
		osmoPrice = osmomath.MustNewBigDecFromStr("0.5")
		atomPrice = osmomath.MustNewBigDecFromStr("7")
		wbtcPrice = osmomath.MustNewBigDecFromStr("50000")

		defaultPriceResult = domain.PricesResult{
			UOSMO: {
				USDC: osmoPrice,
			},
			ATOM: {
				USDC: atomPrice,
			},
			WBTC: {
				USDC: wbtcPrice,
			},
		}

		defaultAmount = sdk.NewInt(1_000_000)

		osmoCoin = sdk.NewCoin(UOSMO, defaultAmount)
		atomCoin = sdk.NewCoin(ATOM, defaultAmount.MulRaw(2))
		wbtcCoin = sdk.NewCoin(WBTC, defaultAmount.MulRaw(3))

		invalidDenom = "invalid"

		osmoCapitalization = osmoPrice.Dec().MulMut(defaultAmount.ToLegacyDec())
		atomCapitalization = atomPrice.Dec().MulMut(defaultAmount.ToLegacyDec().MulInt64(2))
		wbtcCapitalization = wbtcPrice.Dec().MulMut(defaultAmount.ToLegacyDec().MulInt64(3))
		invalidCoin        = sdk.NewCoin(invalidDenom, defaultAmount)

		emptyPrices = domain.PricesResult{}
	)

	tests := []struct {
		name string

		coins              sdk.Coins
		mockedPricesResult domain.PricesResult
		mockedPricesError  error

		expectedError               bool
		expectedAccountCoinsResult  []passthroughdomain.AccountCoinsResult
		expectedTotalCapitalization osmomath.Dec
	}{
		{
			name: "empty coins",

			coins: sdk.Coins{},

			mockedPricesResult: defaultPriceResult,

			expectedAccountCoinsResult:  []passthroughdomain.AccountCoinsResult{},
			expectedTotalCapitalization: osmomath.ZeroDec(),
		},
		{
			name: "one coin in balance",

			coins: sdk.Coins{osmoCoin},

			mockedPricesResult: defaultPriceResult,

			expectedAccountCoinsResult: []passthroughdomain.AccountCoinsResult{
				{
					Coin:                osmoCoin,
					CapitalizationValue: osmoCapitalization,
				},
			},
			expectedTotalCapitalization: osmoCapitalization,
		},
		{
			name: "empty prices -> no capitalization result",

			coins: sdk.Coins{osmoCoin},

			mockedPricesResult: emptyPrices,

			expectedAccountCoinsResult: []passthroughdomain.AccountCoinsResult{
				{
					Coin:                osmoCoin,
					CapitalizationValue: zero,
				},
			},
			expectedTotalCapitalization: zero,
		},
		{
			name: "denom that is not valid denom -> no capitalization",

			coins: sdk.Coins{invalidCoin},

			mockedPricesResult: emptyPrices,

			expectedAccountCoinsResult: []passthroughdomain.AccountCoinsResult{
				{
					Coin:                invalidCoin,
					CapitalizationValue: zero,
				},
			},
			expectedTotalCapitalization: zero,
		},
		{
			name: "multiple coins, including invalid",

			coins: sdk.Coins{osmoCoin, atomCoin, wbtcCoin, invalidCoin},

			mockedPricesResult: defaultPriceResult,

			expectedAccountCoinsResult: []passthroughdomain.AccountCoinsResult{
				{
					Coin:                osmoCoin,
					CapitalizationValue: osmoCapitalization,
				},
				{
					Coin:                atomCoin,
					CapitalizationValue: atomCapitalization,
				},
				{
					Coin:                wbtcCoin,
					CapitalizationValue: wbtcCapitalization,
				},
				{
					Coin:                invalidCoin,
					CapitalizationValue: zero,
				},
			},
			expectedTotalCapitalization: osmoCapitalization.Add(atomCapitalization).Add(wbtcCapitalization),
		},
		{
			name: "error in prices",

			coins: sdk.Coins{osmoCoin},

			mockedPricesError: miscError,

			expectedError: true,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {

			// Set up tokens use case mock with relevant methods
			tokensUsecaseMock := mocks.TokensUsecaseMock{
				GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
					// Return the mocked out results
					return tt.mockedPricesResult, tt.mockedPricesError
				},

				IsValidChainDenomFunc: func(denom string) bool {
					// Treat only UOSMO, ATOM and WBTC as valid for test purposes
					return denom == UOSMO || denom == ATOM || denom == WBTC
				},
			}

			liquidityPricerMock := &mocks.LiquidityPricerMock{
				PriceCoinFunc: func(coin sdk.Coin, price osmomath.BigDec) osmomath.Dec {
					if price.IsZero() {
						return osmomath.ZeroDec()
					}
					return coin.Amount.ToLegacyDec().Mul(price.Dec())
				},
			}

			pu := usecase.NewPassThroughUsecase(nil, nil, &tokensUsecaseMock, liquidityPricerMock, USDC, &log.NoOpLogger{})

			// System under test
			accountCoinsResult, totalCapitalization, err := pu.ComputeCapitalizationForCoins(context.TODO(), tt.coins)

			if tt.expectedError {
				s.Require().Error(err)
				return
			}

			s.Require().NoError(err)

			// Assert
			s.Require().Equal(tt.expectedAccountCoinsResult, accountCoinsResult)
			s.Require().Equal(tt.expectedTotalCapitalization, totalCapitalization)
		})
	}
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

			pu := usecase.NewPassThroughUsecase(&grpcClientMock, &poolsUseCaseMock, nil, nil, USDC, &log.NoOpLogger{})

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

			pu := usecase.NewPassThroughUsecase(&grpcClientMock, &poolsUseCaseMock, nil, nil, USDC, &log.NoOpLogger{})

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

			pu := usecase.NewPassThroughUsecase(nil, &poolsUseCaseMock, nil, nil, USDC, &log.NoOpLogger{})

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
