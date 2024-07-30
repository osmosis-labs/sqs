package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
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

	// TestGetPotrfolioAssets_HappyPath and TestFetchAndAggregateBalancesByUserConcurrent_HappyPath
	// share the test concfiguration and expected results.
	sharedExpectedPortfolioAssetsResult = passthroughdomain.PortfolioAssetsCategoryResult{
		AccountCoinsResult: []passthroughdomain.AccountCoinsResult{
			{
				// Note: 2x osmo from 2 functions
				Coin:                osmoCoin.Add(osmoCoin),
				CapitalizationValue: osmoCapitalization.Add(osmoCapitalization),
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
		Capitalization: osmoCapitalization.Add(osmoCapitalization).Add(atomCapitalization).Add(wbtcCapitalization),
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

	// Initialize GRPC client mock
	grpcClientMock := mocks.PassthroughGRPCClientMock{
		MockAllBalancesCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			if address != defaultAddress {
				return sdk.Coins{}, miscError
			}
			// Note: we return empty coins for simplicity. This method is tested by its individual unit test.
			return sdk.NewCoins(osmoCoin), nil
		},
		MockAccountLockedCoinsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			if address != defaultAddress {
				return sdk.Coins{}, miscError
			}
			// Note: we return empty coins for simplicity. This method is tested by its individual unit test.
			return sdk.Coins{}, nil
		},
		MockDelegatorDelegationsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			if address != defaultAddress {
				return nil, miscError
			}
			return sdk.NewCoins(osmoCoin), nil
		},
		MockDelegatorUnbondingDelegationsCb: func(ctx context.Context, address string) (sdk.Coins, error) {
			if address != defaultAddress {
				return nil, miscError
			}
			// Note that osmo is here again
			return sdk.NewCoins(atomCoin, osmoCoin), nil
		},
		MockUserPositionsBalancesCb: func(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error) {
			if address != defaultAddress {
				return sdk.Coins{}, sdk.Coins{}, miscError
			}
			return sdk.NewCoins(wbtcCoin, invalidCoin), sdk.Coins{}, nil
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
	_, err := pu.GetPortfolioAssets(context.TODO(), defaultAddress)
	s.Require().NoError(err)

	// Assert

	// NOte: below is a hack to avoid code duplication.
	// We preserve the shared values for total value cap and account coins result.
	tempTotalValueCap := sharedExpectedPortfolioAssetsResult.Capitalization
	tempAccountCoinsResult := sharedExpectedPortfolioAssetsResult.AccountCoinsResult

	// Then, we modify per the expectation of this test case:
	// Only the return from balances is considered (osmo) but total capitalization aggregates all outputs (shared capitalization + 1 extra from balances)
	sharedExpectedPortfolioAssetsResult.Capitalization = sharedExpectedPortfolioAssetsResult.Capitalization.Add(osmoCapitalization)
	sharedExpectedPortfolioAssetsResult.AccountCoinsResult = []passthroughdomain.AccountCoinsResult{
		{
			Coin:                osmoCoin,
			CapitalizationValue: osmoCapitalization,
		},
	}

	// Assert the results are correct.
	// TODO:
	// s.validatePortfolioAssetsResult(sharedExpectedPortfolioAssetsResult, actualPortfolioAssets)

	// Switch back to the original values
	sharedExpectedPortfolioAssetsResult.Capitalization = tempTotalValueCap
	sharedExpectedPortfolioAssetsResult.AccountCoinsResult = tempAccountCoinsResult
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
func (s *PassthroughUseCaseTestSuite) validatePortfolioAssetsResult(expectedResult passthroughdomain.PortfolioAssetsCategoryResult, actualResult passthroughdomain.PortfolioAssetsCategoryResult) {
	s.Require().Equal(expectedResult.Capitalization, actualResult.Capitalization)

	// Sort the results for comparison. Order not guaranteed due to concurrency.
	sort.Slice(actualResult.AccountCoinsResult, func(i, j int) bool {
		return actualResult.AccountCoinsResult[i].Coin.Denom < actualResult.AccountCoinsResult[j].Coin.Denom
	})

	sort.Slice(expectedResult.AccountCoinsResult, func(i, j int) bool {
		return expectedResult.AccountCoinsResult[i].Coin.Denom < expectedResult.AccountCoinsResult[j].Coin.Denom
	})

	s.Require().Equal(expectedResult.AccountCoinsResult, actualResult.AccountCoinsResult)
}
