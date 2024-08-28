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
	validGammShareAmount = osmomath.NewInt(3_000_000)

	nonShareDefaultBalances = sdk.NewCoins(
		sdk.NewCoin(UOSMO, osmomath.NewInt(1_000_000)),
		sdk.NewCoin(ATOM, osmomath.NewInt(2_000_000)),
	)

	defaultGammShareCoin = sdk.NewCoin(validGammShareDenom, validGammShareAmount)
	defaultBalances      = nonShareDefaultBalances.Add(defaultGammShareCoin)

	defaultExitPoolCoins = sdk.NewCoins(
		sdk.NewCoin(WBTC, osmomath.NewInt(1_000_000)),
		sdk.NewCoin(ETH, osmomath.NewInt(500_000)),
	)

	defaultConcentratedCoin = sdk.NewCoin(USDC, osmomath.NewInt(1_000_000))

	defaultConcentratedShareCoin = sdk.NewCoin(usecase.ConcentratedSharePrefix+"/pool", osmomath.NewInt(1_000_000))

	emptyCoins = sdk.Coins{}

	zero = osmomath.ZeroDec()

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

	defaultAmount = osmomath.NewInt(1_000_000)

	osmoCoin = sdk.NewCoin(UOSMO, defaultAmount)
	atomCoin = sdk.NewCoin(ATOM, defaultAmount.MulRaw(2))
	wbtcCoin = sdk.NewCoin(WBTC, defaultAmount.MulRaw(3))

	invalidDenom = "invalid"

	osmoCapitalization = osmoPrice.Dec().MulMut(defaultAmount.ToLegacyDec())
	atomCapitalization = atomPrice.Dec().MulMut(defaultAmount.ToLegacyDec().MulInt64(2))
	wbtcCapitalization = wbtcPrice.Dec().MulMut(defaultAmount.ToLegacyDec().MulInt64(3))
	invalidCoin        = sdk.NewCoin(invalidDenom, defaultAmount)

	emptyPrices = domain.PricesResult{}

	////////////////////////////
	// Mocks

	liquidityPricerMock = &mocks.LiquidityPricerMock{
		PriceCoinFunc: func(coin sdk.Coin, price osmomath.BigDec) osmomath.Dec {
			if price.IsZero() {
				return osmomath.ZeroDec()
			}
			return coin.Amount.ToLegacyDec().Mul(price.Dec())
		},
	}

	isValidChainDenomFuncMock = func(denom string) bool {
		// Treat only UOSMO, ATOM and WBTC as valid for test purposes
		return denom == UOSMO || denom == ATOM || denom == WBTC
	}
)

func TestPassthroughUseCase(t *testing.T) {
	suite.Run(t, new(PassthroughUseCaseTestSuite))
}

// Tests the happy path of get portfolio assets byusing mocks.
// It sets up several fetch functions where some return multiple coins and others contain invalid denoms.
// Eventually, it asserts that the expected results match actual, aggregating balances and computing the total
// capitalization.
func (s *PassthroughUseCaseTestSuite) TestGetPotrfolioAssets_HappyPath() {
	// Set up tokens use case mock with relevant methods
	tokensUsecaseMock := mocks.TokensUsecaseMock{
		GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
			// Return the mocked out results
			return defaultPriceResult, nil
		},

		IsValidChainDenomFunc: isValidChainDenomFuncMock,
	}

	var (
		miscError = fmt.Errorf("misc error")
	)

	// Initialize GRPC client mock
	grpcClientMock := mocks.PassthroughGRPCClientMock{
		MockAllBalancesCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			// Return coins and no error.
			return sdk.NewCoins(osmoCoin), nil
		},
		MockAccountLockedCoinsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			// Note: we return empty coins for simplicity. This method is tested by its individual unit test.
			// Returns an error to test the silent error handling.
			return sdk.Coins{}, miscError
		},
		MockAccountUnlockingCoinsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			// Note: we return empty coins for simplicity. This method is tested by its individual unit test.
			// Returns an error to test the silent error handling.
			return sdk.Coins{}, miscError
		},
		MockDelegatorDelegationsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			// Return error to test the silent error handling.
			return sdk.NewCoins(osmoCoin), miscError
		},
		MockDelegatorUnbondingDelegationsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			// Return error to test the silent error handling.
			return sdk.NewCoins(atomCoin, osmoCoin), miscError
		},
		MockUserPositionsBalancesCb: func(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error) {
			// Return error to test the silent error handling.
			return sdk.NewCoins(wbtcCoin), sdk.NewCoins(invalidCoin), miscError
		},
		MockDelegationRewardsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			// Return error to test the silent error handling.
			return sdk.NewCoins(osmoCoin), miscError
		},
	}

	// Initialize pools use case mock
	poolsUseCaseMock := mocks.PoolsUsecaseMock{
		CalcExitCFMMPoolFunc: func(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error) {
			// Note: we return empty coins for simplicity. This method is tested by its individual unit test.
			return sdk.Coins{}, nil
		},
	}

	pu := usecase.NewPassThroughUsecase(&grpcClientMock, &poolsUseCaseMock, &tokensUsecaseMock, liquidityPricerMock, USDC, &log.NoOpLogger{})

	// System under test
	actualPortfolioAssets, err := pu.GetPortfolioAssets(context.TODO(), defaultAddress)
	s.Require().NoError(err)

	// Assert

	// Expected results are manually calculated based on the mocked out results.
	expectedResult := passthroughdomain.PortfolioAssetsResult{
		Categories: map[string]passthroughdomain.PortfolioAssetsCategoryResult{
			usecase.UserBalancesAssetsCategoryName: {
				Capitalization: osmoCapitalization,
				AccountCoinsResult: []passthroughdomain.AccountCoinsResult{
					{
						Coin:                osmoCoin,
						CapitalizationValue: osmoCapitalization,
					},
				},
			},
			usecase.UnstakingAssetsCategoryName: {
				Capitalization: osmoCapitalization.Add(atomCapitalization),
				IsBestEffort:   true,
			},
			usecase.StakedAssetsCategoryName: {
				Capitalization: osmoCapitalization,
				IsBestEffort:   true,
			},
			usecase.InLocksAssetsCategoryName: {
				Capitalization: zero,
				IsBestEffort:   true,
			},
			usecase.PooledAssetsCategoryName: {
				Capitalization: wbtcCapitalization,
				IsBestEffort:   true,
			},
			usecase.UnclaimedRewardsAssetsCategoryName: {
				Capitalization: osmoCapitalization,
				IsBestEffort:   true,
			},
			usecase.TotalAssetsCategoryName: {
				Capitalization: osmoCapitalization.Add(osmoCapitalization).Add(osmoCapitalization).Add(atomCapitalization).Add(wbtcCapitalization).Add(osmoCapitalization),
				AccountCoinsResult: []passthroughdomain.AccountCoinsResult{
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
					{
						Coin:                osmoCoin.Add(osmoCoin).Add(osmoCoin).Add(osmoCoin),
						CapitalizationValue: osmoCapitalization.Add(osmoCapitalization).Add(osmoCapitalization).Add(osmoCapitalization),
					},
				},
				IsBestEffort: true,
			},
		},
	}

	// Assert the results are correct.
	s.validatePortfolioAssetsResult(expectedResult, actualPortfolioAssets)
}

// Tests the compute capitalization for coins method using mocks.
func (s *PassthroughUseCaseTestSuite) TestComputeCapitalizationForCoins() {
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

				IsValidChainDenomFunc: isValidChainDenomFuncMock,
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

// Tests the get locked and unlocking coins method using mocks.
func (s *PassthroughUseCaseTestSuite) TestGetCoinsFromLocks() {
	dafultResult := nonShareDefaultBalances.Add(defaultExitPoolCoins...)

	tests := []struct {
		name    string
		address string

		mockAccountLockedCoinsIfDefaultAddress    sdk.Coins
		mockAccountUnlockingCoinsIfDefaultAddress sdk.Coins

		expectedCoins sdk.Coins
		expectedError error
	}{
		{
			name: "happy path",

			address: defaultAddress,

			mockAccountLockedCoinsIfDefaultAddress: defaultBalances,

			expectedCoins: dafultResult,
		},
		{
			name: "happy path with unlocking",

			address: defaultAddress,

			mockAccountLockedCoinsIfDefaultAddress:    defaultBalances,
			mockAccountUnlockingCoinsIfDefaultAddress: defaultBalances,

			// 2x for locked and unlocking.
			expectedCoins: dafultResult.Add(dafultResult...),
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
			expectedCoins: sdk.Coins{},
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
				MockAccountUnlockingCoinsCb: func(ctx context.Context, address string) (sdk.Coins, error) {

					// If not default address, return grpc client error
					if address != defaultAddress {
						return sdk.Coins{}, grpcClientError
					}

					if tt.mockAccountUnlockingCoinsIfDefaultAddress == nil {
						return sdk.Coins{}, nil
					}

					// If default address, return mock balances
					return tt.mockAccountUnlockingCoinsIfDefaultAddress, nil
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
			actualBalances, err := pu.GetCoinsFromLocks(context.TODO(), tt.address)

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

		expectedBalanceCoins sdk.Coins
		expectedShareCoins   sdk.Coins
		expectedError        error
	}{
		{
			name: "happy path",

			address: defaultAddress,

			mockAllBalancesIfDefaultAddress: defaultBalances,

			expectedBalanceCoins: nonShareDefaultBalances,
			expectedShareCoins:   defaultExitPoolCoins,
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
			expectedBalanceCoins: nonShareDefaultBalances,
			expectedShareCoins:   sdk.Coins{},
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
			actualBalances, gammShareBalances, err := pu.GetBankBalances(context.TODO(), tt.address)

			// Assert
			s.Require().Equal(tt.expectedBalanceCoins, actualBalances)
			s.Require().Equal(tt.expectedError, err)
			s.Require().Equal(tt.expectedShareCoins, gammShareBalances)
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

// validatePortfolioAssetsResult validates the expected and actual portfolio assets results.
func (s *PassthroughUseCaseTestSuite) validatePortfolioAssetsResult(expectedResult passthroughdomain.PortfolioAssetsResult, actualResult passthroughdomain.PortfolioAssetsResult) {

	s.Require().Equal(len(expectedResult.Categories), len(actualResult.Categories))

	for categoryName, expectedCategory := range expectedResult.Categories {
		actualCategory := actualResult.Categories[categoryName]

		s.Require().Equal(expectedCategory.Capitalization, actualCategory.Capitalization, categoryName)
		s.Require().Equal(len(expectedCategory.AccountCoinsResult), len(actualCategory.AccountCoinsResult), categoryName)
		for j, expectedAccountCoinsResult := range expectedCategory.AccountCoinsResult {
			actualAccountCoinsResult := actualCategory.AccountCoinsResult[j]

			s.Require().Equal(expectedAccountCoinsResult.Coin, actualAccountCoinsResult.Coin, categoryName)
			s.Require().Equal(expectedAccountCoinsResult.CapitalizationValue, actualAccountCoinsResult.CapitalizationValue, categoryName)
		}

		s.Require().Equal(expectedCategory.IsBestEffort, actualCategory.IsBestEffort, categoryName)
	}
}
