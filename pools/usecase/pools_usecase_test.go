package usecase_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
	"github.com/stretchr/testify/suite"

	cosmwasmpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/stableswap"
	gammtypes "github.com/osmosis-labs/osmosis/v28/x/gamm/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	sqspassthroughdomain "github.com/osmosis-labs/sqs/sqsdomain/passthroughdomain"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	v1beta1 "github.com/osmosis-labs/sqs/pkg/api/v1beta1"
	"github.com/osmosis-labs/sqs/pools/usecase"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
)

type PoolsUsecaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultPoolID = uint64(1)
)

var (
	denomOne   = routertesting.DenomOne
	denomTwo   = routertesting.DenomTwo
	denomThree = routertesting.DenomThree
	denomFour  = routertesting.DenomFour
	denomFive  = routertesting.DenomFive

	defaultTakerFee = routertesting.DefaultTakerFee

	defaultAmt0 = routertesting.DefaultAmt0
	defaultAmt1 = routertesting.DefaultAmt1

	defaultPoolLiquidityCap = osmomath.NewInt(100)

	// Default values
	defaultTime  = time.Unix(0, 0)
	defaultError = fmt.Errorf("forced error")

	// Default APR and fee data
	defaultAPRData = sqspassthroughdomain.PoolAPRDataStatusWrap{PoolAPR: sqspassthroughdomain.PoolAPR{
		PoolID: defaultPoolID,
		SwapFees: sqspassthroughdomain.PoolDataRange{
			Lower: 0.01,
			Upper: 0.02,
		},
	}}
	defaultFeeData = sqspassthroughdomain.PoolFeesDataStatusWrap{
		PoolFee: sqspassthroughdomain.PoolFee{
			PoolID: fmt.Sprintf("%d", defaultPoolID),
		},
	}
)

func TestPoolsUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(PoolsUsecaseTestSuite))
}

// Validates that candidate routes are correctly converted into routes with all the pool data.
// Check that:
// - pool data is correctly set on routable pools.
// - taker fee is correctly set.
// - token out denom is correctly set.
func (s *PoolsUsecaseTestSuite) TestGetRoutesFromCandidates() {

	s.Setup()

	// Setup default chain pool
	poolID := s.PrepareBalancerPoolWithCoins(sdk.NewCoin(denomOne, defaultAmt0), sdk.NewCoin(denomTwo, defaultAmt1))
	balancerPool, err := s.App.GAMMKeeper.GetPool(s.Ctx, poolID)
	s.Require().NoError(err)

	defaultPool := &mocks.MockRoutablePool{
		ChainPoolModel: balancerPool,
		ID:             defaultPoolID,
	}

	validPools := []sqsdomain.PoolI{
		defaultPool,
	}

	// We break the pool by changing the pool type
	// to the wrong type. Note that the default is balancer.
	brokenChainPool := *defaultPool
	brokenChainPool.PoolType = poolmanagertypes.CosmWasm

	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	_, err = pools.NewRoutablePool(&brokenChainPool, denomTwo, defaultTakerFee, cosmWasmPoolsParams)
	// Validate that it is indeed broken.
	s.Require().Error(err)

	validCandidateRoutes := sqsdomain.CandidateRoutes{
		Routes: []sqsdomain.CandidateRoute{
			{
				Pools: []sqsdomain.CandidatePool{
					{
						ID:            defaultPoolID,
						TokenOutDenom: denomTwo,
					},
				},
			},
		},
	}

	validTakerFeeMap := sqsdomain.TakerFeeMap{
		sqsdomain.DenomPair{
			Denom0: denomOne,
			Denom1: denomTwo,
		}: defaultTakerFee,
	}

	tests := []struct {
		name string

		pools           []sqsdomain.PoolI
		candidateRoutes sqsdomain.CandidateRoutes
		takerFeeMap     sqsdomain.TakerFeeMap
		tokenInDenom    string
		tokenOutDenom   string

		expectedError error

		expectedRoutes []route.RouteImpl
	}{
		{
			name:  "valid conversion of single route",
			pools: validPools,

			candidateRoutes: validCandidateRoutes,
			takerFeeMap:     validTakerFeeMap,

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedRoutes: []route.RouteImpl{
				{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(defaultPool, denomTwo, defaultTakerFee),
					},
				},
			},
		},
		{
			name:  "no taker fee - use default",
			pools: validPools,

			candidateRoutes: validCandidateRoutes,

			// empty map
			takerFeeMap: sqsdomain.TakerFeeMap{},

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedRoutes: []route.RouteImpl{
				{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(defaultPool, denomTwo, sqsdomain.DefaultTakerFee),
					},
				},
			},
		},
		{
			name:  "error: no pool in state",
			pools: []sqsdomain.PoolI{},

			candidateRoutes: validCandidateRoutes,

			// empty map
			takerFeeMap: validTakerFeeMap,

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedError: domain.PoolNotFoundError{
				PoolID: defaultPoolID,
			},
		},
		{
			name:  "broken chain pool is skipped without failing the whole conversion",
			pools: []sqsdomain.PoolI{&brokenChainPool, defaultPool},

			candidateRoutes: validCandidateRoutes,
			takerFeeMap:     validTakerFeeMap,

			tokenInDenom:  denomOne,
			tokenOutDenom: denomTwo,

			expectedRoutes: []route.RouteImpl{
				{
					Pools: []domain.RoutablePool{
						s.newRoutablePool(defaultPool, denomTwo, defaultTakerFee),
					},
				},
			},
		},

		// TODO:
		// Valid conversion of single multi-hop route
		// Valid conversion of two routes where one is multi hop
	}

	for _, tc := range tests {
		tc := tc
		s.Run(tc.name, func() {
			logger := &log.NoOpLogger{}
			// Create router repository
			routerRepo := routerrepo.New(&log.NoOpLogger{})
			routerRepo.SetTakerFees(tc.takerFeeMap)

			// Create pools use case
			poolsUsecase, err := usecase.NewPoolsUsecase(&domain.PoolsConfig{}, "node-uri-placeholder", routerRepo, domain.UnsetScalingFactorGetterCb, nil, logger)
			s.Require().NoError(err)

			poolsUsecase.StorePools(tc.pools)

			// System under test
			actualRoutes, err := poolsUsecase.GetRoutesFromCandidates(tc.candidateRoutes, tc.tokenInDenom, tc.tokenOutDenom)

			if tc.expectedError != nil {
				s.Require().Error(err)
				s.Require().Equal(tc.expectedError, err)
				return
			}

			s.Require().NoError(err)

			// Validate routes
			s.Require().Equal(len(tc.expectedRoutes), len(actualRoutes))
			for i, actualRoute := range actualRoutes {
				expectedRoute := tc.expectedRoutes[i]

				// Note: this is only done to be able to use the ValidateRoutePools
				// helper method for validation.
				// Note token in is chosen arbitrarily since it is irrelevant for this test
				tokenIn := sdk.NewCoin(tc.tokenInDenom, osmomath.NewInt(100))
				actualPools, _, _, err := actualRoute.PrepareResultPools(context.TODO(), tokenIn, logger)
				s.Require().NoError(err)
				expectedPools, _, _, err := expectedRoute.PrepareResultPools(context.TODO(), tokenIn, logger)
				s.Require().NoError(err)

				// Validates:
				// 1. Correct pool data
				// 2. Correct taker fee
				// 3. Correct token out denom
				s.ValidateRoutePools(expectedPools, actualPools)
			}
		})
	}
}

func (s *PoolsUsecaseTestSuite) TestProcessOrderbookPoolIDForBaseQuote() {
	const (
		differentPoolID        = defaultPoolID + 1
		defaultContractAddress = "default-address"
	)

	testCases := []struct {
		name                        string
		base                        string
		quote                       string
		poolID                      uint64
		poolLiquidityCapitalization osmomath.Int

		preStoreValidEntryCap osmomath.Int
		preStoreInvalidEntry  bool

		expectedError   bool
		expectedUpdated bool

		expectedCanonicalOrderbookPoolID uint64
		expectedContractAddress          string
	}{
		{
			name:  "valid entry - no pre set",
			base:  denomOne,
			quote: denomTwo,

			poolID:                      defaultPoolID,
			poolLiquidityCapitalization: defaultPoolLiquidityCap,

			expectedUpdated:                  true,
			expectedCanonicalOrderbookPoolID: defaultPoolID,
			expectedContractAddress:          defaultContractAddress,
		},
		{
			name:  "valid entry - pre set with smaller cap -> overriden",
			base:  denomOne,
			quote: denomTwo,

			poolID:                      defaultPoolID,
			poolLiquidityCapitalization: defaultPoolLiquidityCap,

			preStoreValidEntryCap: defaultPoolLiquidityCap.Sub(osmomath.OneInt()),

			expectedUpdated:                  true,
			expectedCanonicalOrderbookPoolID: defaultPoolID,
			expectedContractAddress:          defaultContractAddress,
		},
		{
			name:  "valid entry - pre set with larger cap -> not overriden",
			base:  denomOne,
			quote: denomTwo,

			poolID:                      defaultPoolID,
			poolLiquidityCapitalization: defaultPoolLiquidityCap,

			preStoreValidEntryCap: defaultPoolLiquidityCap.Add(osmomath.OneInt()),

			expectedUpdated:                  false,
			expectedCanonicalOrderbookPoolID: differentPoolID,
			expectedContractAddress:          usecase.OriginalOrderbookAddress,
		},
		{
			name:  "invalid entry - pre set with larger cap -> not overriden",
			base:  denomOne,
			quote: denomTwo,

			poolID:                      defaultPoolID,
			poolLiquidityCapitalization: defaultPoolLiquidityCap,

			preStoreInvalidEntry: true,

			expectedError: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {

			poolsUsecase := s.newDefaultPoolsUseCase()

			// Pre-set invalid data for the base/quote
			if tc.preStoreInvalidEntry {
				poolsUsecase.StoreInvalidOrderBookEntry(tc.base, tc.quote)
			}

			// Pre-set valid data for the base/quote
			if !tc.preStoreValidEntryCap.IsNil() {
				// Note that we store the entry with different pool ID to make sure that the
				// poolID is updated to the new value.
				poolsUsecase.StoreValidOrdeBookEntry(tc.base, tc.quote, differentPoolID, tc.preStoreValidEntryCap)
			}

			// System under test
			updatedBool, err := poolsUsecase.ProcessOrderbookPoolIDForBaseQuote(tc.base, tc.quote, tc.poolID, tc.poolLiquidityCapitalization, defaultContractAddress)

			if tc.expectedError {
				s.Require().Error(err)
				return
			}

			s.Require().NoError(err)
			s.Require().Equal(tc.expectedUpdated, updatedBool)

			canonicalPoolID, contractAddress, err := poolsUsecase.GetCanonicalOrderbookPool(tc.base, tc.quote)
			s.Require().NoError(err)

			s.Require().Equal(tc.expectedCanonicalOrderbookPoolID, canonicalPoolID)
			s.Require().Equal(tc.expectedContractAddress, contractAddress)

			// Validate that the canonical orderbook pool ID is correctly set
			s.Require().Equal(tc.expectedCanonicalOrderbookPoolID == defaultPoolID, poolsUsecase.IsCanonicalOrderbookPool(defaultPoolID))
			s.Require().Equal(tc.expectedCanonicalOrderbookPoolID == differentPoolID, poolsUsecase.IsCanonicalOrderbookPool(differentPoolID))
		})
	}
}

// Happy path test for StorePools validating that
// for orderbook pools, we also update the canonical orderbook pool ID.
// We also validate that any errors stemming from orderbook handling logic are silently skipped
func (s *PoolsUsecaseTestSuite) TestStorePools() {

	const (
		validOrderBookPoolID   = defaultPoolID + 1
		invalidOrderBookPoolID = defaultPoolID + 2

		imaginaryAddress = "imaginary-address"
	)

	var (
		defaultBalancerPool = &mocks.MockRoutablePool{
			ChainPoolModel: &mocks.ChainPoolMock{
				ID:   defaultPoolID,
				Type: poolmanagertypes.Balancer,
			},
			ID: defaultPoolID,
		}

		validBaseDenom      = denomOne
		orderBookQuoteDenom = denomTwo

		invalidBaseDenom = denomThree

		defaultOrderbookContractInfo = cosmwasmpool.ContractInfo{
			Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME,
			Version:  cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
		}

		validOrderBookPool = &mocks.MockRoutablePool{
			ChainPoolModel: &cosmwasmpoolmodel.CosmWasmPool{
				PoolId:          defaultPoolID + 1,
				ContractAddress: imaginaryAddress,
			},
			ID: defaultPoolID + 1,
			CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
				ContractInfo: defaultOrderbookContractInfo,

				Data: cosmwasmpool.CosmWasmPoolData{
					Orderbook: &cosmwasmpool.OrderbookData{
						BaseDenom:  validBaseDenom,
						QuoteDenom: orderBookQuoteDenom,
					},
				},
			},
		}

		invalidOrderBookPool = &mocks.MockRoutablePool{
			ChainPoolModel: &cosmwasmpoolmodel.CosmWasmPool{
				PoolId:          defaultPoolID + 2,
				ContractAddress: imaginaryAddress,
			},
			ID: defaultPoolID + 2,
			CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
				ContractInfo: defaultOrderbookContractInfo,

				Data: cosmwasmpool.CosmWasmPoolData{
					Orderbook: &cosmwasmpool.OrderbookData{
						BaseDenom:  invalidBaseDenom,
						QuoteDenom: orderBookQuoteDenom,
					},
				},
			},
		}

		validPools = []sqsdomain.PoolI{
			defaultBalancerPool,
			validOrderBookPool,
			invalidOrderBookPool,
		}
	)

	poolsUsecase := s.newDefaultPoolsUseCase()

	// Pre-set invalid data for the base/quote
	poolsUsecase.StoreInvalidOrderBookEntry(invalidBaseDenom, orderBookQuoteDenom)

	// System under test
	poolsUsecase.StorePools(validPools)

	// Validate that the pools are stored
	actualBalancerPool, err := poolsUsecase.GetPool(defaultPoolID)
	s.Require().NoError(err)
	s.Require().Equal(defaultBalancerPool, actualBalancerPool)

	actualOrderBookPool, err := poolsUsecase.GetPool(validOrderBookPoolID)
	s.Require().NoError(err)
	s.Require().Equal(validOrderBookPool, actualOrderBookPool)

	// Validate that the canonical orderbook pool ID is correctly set
	canonicalPoolID, orderbookAddress, err := poolsUsecase.GetCanonicalOrderbookPool(validBaseDenom, orderBookQuoteDenom)
	s.Require().NoError(err)
	s.Require().Equal(validOrderBookPool.ID, canonicalPoolID)
	s.Require().Equal(imaginaryAddress, orderbookAddress)

	// Validae that the invalid orderbook is saved as the pool but it is not used for the canonical orderbook pool ID
	actualOrderBookPool, err = poolsUsecase.GetPool(invalidOrderBookPoolID)
	s.Require().NoError(err)
	s.Require().Equal(invalidOrderBookPool, actualOrderBookPool)

	_, _, err = poolsUsecase.GetCanonicalOrderbookPool(invalidBaseDenom, orderBookQuoteDenom)
	s.Require().Error(err)
}

// This test validates that the canonical orderbook pool IDs are returned as intended
// if they are correctly set. The correctness of setting them is ensured
// by the StorePools and ProcessOrderbookPoolIDForBaseQuote tests.
func (s *PoolsUsecaseTestSuite) TestGetAllCanonicalOrderbooks_HappyPath() {

	poolsUseCase := s.newDefaultPoolsUseCase()

	// Denom one and denom two
	poolsUseCase.StoreValidOrdeBookEntry(denomOne, denomTwo, defaultPoolID, defaultPoolLiquidityCap)

	// Denom three and denom four
	poolsUseCase.StoreValidOrdeBookEntry(denomThree, denomFour, defaultPoolID+1, defaultPoolLiquidityCap.Add(osmomath.OneInt()))

	expectedCanonicalOrderbookPoolIDs := []domain.CanonicalOrderBooksResult{
		{
			Base:            denomOne,
			Quote:           denomTwo,
			PoolID:          defaultPoolID,
			ContractAddress: usecase.OriginalOrderbookAddress,
		},
		{
			Base:            denomThree,
			Quote:           denomFour,
			PoolID:          defaultPoolID + 1,
			ContractAddress: usecase.OriginalOrderbookAddress,
		},
	}

	// System under test
	canonicalOrderbooks, err := poolsUseCase.GetAllCanonicalOrderbookPoolIDs()
	s.Require().NoError(err)

	// Validate that the correct number of canonical orderbook pool IDs are returned
	s.Require().Equal(len(canonicalOrderbooks), 2)

	// Validate that the correct canonical orderbook pool IDs are returned
	s.Require().Equal(expectedCanonicalOrderbookPoolIDs, canonicalOrderbooks)

}

// Happy path test to vaidate that no panics/errors occur and coins are returned
// as intended.
// The correctness of math is ensured at a different layer of abstraction.
func (s *PoolsUsecaseTestSuite) TestCalcExitCFMMPool_HappyPath() {

	s.Setup()

	// Create pool
	poolID := s.PrepareBalancerPool()
	cfmmPool, err := s.App.GAMMKeeper.GetCFMMPool(s.Ctx, poolID)
	s.Require().NoError(err)

	// Get balances
	poolBalances := s.App.BankKeeper.GetAllBalances(s.Ctx, cfmmPool.GetAddress())
	s.Require().NoError(err)

	// Create sqs pool
	sqsPool := sqsdomain.NewPool(cfmmPool, cfmmPool.GetSpreadFactor(s.Ctx), poolBalances)

	// Create default use case
	poolsUseCase := s.newDefaultPoolsUseCase()

	// Store pool
	poolsUseCase.StorePools([]sqsdomain.PoolI{sqsPool})

	// Arbitrary large number.
	numSharesExiting := osmomath.NewInt(1_000_000_000_000_000_000)

	// System under test
	actualCoins, err := poolsUseCase.CalcExitCFMMPool(poolID, numSharesExiting)

	// Validate
	s.Require().NoError(err)
	s.Require().False(actualCoins.Empty())
}

// TestCalcExitPool is a copy of node's TestCalcExitPool
// This implementation includes several specific test cases to cover arithmetic on floats instead of sdk math data types.
// @link https://github.com/osmosis-labs/osmosis/blob/fde1776476d9c2f849dcbfb30ca3ec64d0e12863/x/gamm/pool-models/internal/cfmm_common/lp_test.go#L31
func (s *PoolsUsecaseTestSuite) TestCalcExitPool() {
	emptyContext := sdk.Context{}

	twoStablePoolAssets := sdk.NewCoins(
		sdk.NewInt64Coin("foo", 1000000000),
		sdk.NewInt64Coin("bar", 1000000000),
	)

	threeBalancerPoolAssets := []balancer.PoolAsset{
		{Token: sdk.NewInt64Coin("foo", 20000000000000), Weight: osmomath.NewIntFromUint64(5)},
		{Token: sdk.NewInt64Coin("bar", 30000000000000), Weight: osmomath.NewIntFromUint64(5)},
		{Token: sdk.NewInt64Coin("baz", 40000000000000), Weight: osmomath.NewIntFromUint64(5)},
	}

	fourthBalancerPoolAssets := []balancer.PoolAsset{
		{Token: sdk.NewInt64Coin("foo", 9000000000000000000).AddAmount(osmomath.NewInt(9000000000000000000)), Weight: osmomath.NewIntFromUint64(5)},
	}

	// create these pools used for testing
	twoAssetPool, err := stableswap.NewStableswapPool(
		1,
		stableswap.PoolParams{ExitFee: osmomath.ZeroDec()},
		twoStablePoolAssets,
		[]uint64{1, 1},
		"",
		"",
	)
	s.Assert().NoError(err)

	threeAssetPool, err := balancer.NewBalancerPool(
		1,
		balancer.PoolParams{SwapFee: osmomath.ZeroDec(), ExitFee: osmomath.ZeroDec()},
		threeBalancerPoolAssets,
		"",
		time.Now(),
	)
	s.Assert().NoError(err)

	twoAssetPoolWithExitFee, err := stableswap.NewStableswapPool(
		1,
		stableswap.PoolParams{ExitFee: osmomath.MustNewDecFromStr("0.0001")},
		twoStablePoolAssets,
		[]uint64{1, 1},
		"",
		"",
	)
	s.Assert().NoError(err)

	threeAssetPoolWithExitFee, err := balancer.NewBalancerPool(
		1,
		balancer.PoolParams{SwapFee: osmomath.ZeroDec(), ExitFee: osmomath.MustNewDecFromStr("0.0002")},
		threeBalancerPoolAssets,
		"",
		time.Now(),
	)
	s.Assert().NoError(err)

	fourthAssetPoolWithExitFee, err := balancer.NewBalancerPool(
		1,
		balancer.PoolParams{SwapFee: osmomath.ZeroDec(), ExitFee: osmomath.MustNewDecFromStr("0.0002")},
		fourthBalancerPoolAssets,
		"",
		time.Now(),
	)
	s.Assert().NoError(err)

	tests := []struct {
		name          string
		pool          gammtypes.CFMMPoolI
		exitingShares osmomath.Int
		expError      bool
	}{
		{
			name:          "two-asset pool, exiting shares grater than total shares",
			pool:          &twoAssetPool,
			exitingShares: twoAssetPool.GetTotalShares().AddRaw(1),
			expError:      true,
		},
		{
			name:          "three-asset pool, exiting shares grater than total shares",
			pool:          &threeAssetPool,
			exitingShares: threeAssetPool.GetTotalShares().AddRaw(1),
			expError:      true,
		},
		{
			name:          "two-asset pool, valid exiting shares",
			pool:          &twoAssetPool,
			exitingShares: twoAssetPool.GetTotalShares().QuoRaw(2),
			expError:      false,
		},
		{
			name:          "three-asset pool, valid exiting shares",
			pool:          &threeAssetPool,
			exitingShares: osmomath.NewIntFromUint64(3000000000000),
			expError:      false,
		},
		{
			name:          "two-asset pool with exit fee, valid exiting shares",
			pool:          &twoAssetPoolWithExitFee,
			exitingShares: twoAssetPoolWithExitFee.GetTotalShares().QuoRaw(2),
			expError:      false,
		},
		{
			name:          "three-asset pool with exit fee, valid exiting shares",
			pool:          &threeAssetPoolWithExitFee,
			exitingShares: osmomath.NewIntFromUint64(7000000000000),
			expError:      false,
		},
		{
			name:          "fourth-asset pool with exit fee, valid exiting shares, overflow amount",
			pool:          &fourthAssetPoolWithExitFee,
			exitingShares: osmomath.NewIntFromUint64(7000000000000),
			expError:      false,
		},
	}

	// Create default use case
	poolsUseCase := s.newDefaultPoolsUseCase()

	for _, test := range tests {
		// using empty context since, currently, the context is not used anyway. This might be changed in the future

		exitFee := test.pool.GetExitFee(emptyContext)
		exitCoins, err := poolsUseCase.CalcExitPool(emptyContext, test.pool, test.exitingShares, exitFee)
		if test.expError {
			s.Require().Error(err, "test: %v", test.name)
		} else {
			s.Require().NoError(err, "test: %v", test.name)

			// exitCoins = ( (1 - exitFee) * exitingShares / poolTotalShares ) * poolTotalLiquidity
			expExitCoins := mulCoins(test.pool.GetTotalPoolLiquidity(emptyContext), (osmomath.OneDec().Sub(exitFee)).MulInt(test.exitingShares).QuoInt(test.pool.GetTotalShares()))
			s.Assert().Equal(expExitCoins.Sort().String(), exitCoins.Sort().String(), "test: %v", test.name)
		}
	}
}

// a helper function used to multiply coins
func mulCoins(coins sdk.Coins, multiplier osmomath.Dec) sdk.Coins {
	outCoins := sdk.Coins{}
	for _, coin := range coins {
		outCoin := sdk.NewCoin(coin.Denom, multiplier.MulInt(coin.Amount).TruncateInt())
		if !outCoin.Amount.IsZero() {
			outCoins = append(outCoins, outCoin)
		}
	}
	return outCoins
}

func (s *PoolsUsecaseTestSuite) TestGetPools() {
	mainnetState := s.SetupMainnetState()
	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	// Define test cases
	tests := []struct {
		name         string
		options      []domain.PoolsOption
		expectedLen  int
		minExpected  int // Minimum expected pools count (for > tests)
		expectError  bool
		validateFunc func(s *PoolsUsecaseTestSuite, pools []sqsdomain.PoolI)
	}{
		{
			name:         "No filter",
			options:      nil,
			expectedLen:  0,    // Not validated strictly
			minExpected:  1500, // Expect more than 1500 pools
			expectError:  false,
			validateFunc: nil,
		},
		{
			name:         "Pool ID filter",
			options:      []domain.PoolsOption{domain.WithPoolIDFilter([]uint64{32, 1, 1066})},
			expectedLen:  3,
			expectError:  false,
			validateFunc: nil,
		},
		{
			name:         "Incentive filter",
			options:      []domain.PoolsOption{domain.WithMarketIncentives(true)},
			expectedLen:  1957,
			expectError:  false,
			validateFunc: nil,
		},
		{
			name:         "Denom filter ( multi denom )",
			options:      []domain.PoolsOption{domain.WithDenom([]string{"osmo", "atom"})},
			expectedLen:  1099,
			expectError:  false,
			validateFunc: nil,
		},
		{
			name:         "Min liquidity cap filter",
			options:      []domain.PoolsOption{domain.WithMinPoolsLiquidityCap(1_000_000)},
			expectedLen:  0, // Not validated strictly
			minExpected:  0, // Expect less than 100 pools
			expectError:  false,
			validateFunc: nil,
		},
		{
			name: "Min liquidity cap and pool ID filter",
			options: []domain.PoolsOption{
				domain.WithMinPoolsLiquidityCap(1),
				domain.WithPoolIDFilter([]uint64{32, 1, 1066}),
			},
			expectedLen: 2,
			expectError: false,
			validateFunc: func(s *PoolsUsecaseTestSuite, pools []sqsdomain.PoolI) {
				s.Require().Contains([]uint64{1, 1066}, pools[0].GetId())
				s.Require().Contains([]uint64{1, 1066}, pools[1].GetId())
			},
		},
		{
			name:         "Empty filter",
			options:      []domain.PoolsOption{domain.WithPoolIDFilter([]uint64{})},
			expectedLen:  0,
			expectError:  false,
			validateFunc: nil,
		},
		{
			name: "Exact match search filter: Pool ID",
			options: []domain.PoolsOption{
				domain.WithSearch("143"), // Pool ID
			},
			expectedLen: 1,
			expectError: false,
			validateFunc: func(s *PoolsUsecaseTestSuite, pools []sqsdomain.PoolI) {
				for _, v := range pools {
					s.Require().Equal(uint64(143), v.GetId())
				}
			},
		},
		{
			name: "Exact match search filter: Denom",
			options: []domain.PoolsOption{
				domain.WithSearch("jitoSOL.pica"),
			},
			expectedLen: 1,
			expectError: false,
			validateFunc: func(s *PoolsUsecaseTestSuite, pools []sqsdomain.PoolI) {
				for _, v := range pools {
					s.Require().Equal(
						// GetPoolDenoms return non-human denom
						"ibc/9A83BDF4C8C5FFDDE735533BC8CD4363714A6474AED1C2C492FB003BB77C7982",
						v.GetPoolDenoms()[0],
					)
				}
			},
		},
		{
			name: "Partial match search filter: Denom",
			options: []domain.PoolsOption{
				domain.WithSearch("has"),
			},
			expectedLen: 4,
			expectError: false,
			validateFunc: func(s *PoolsUsecaseTestSuite, pools []sqsdomain.PoolI) {
				for _, v := range pools {
					s.Require().Contains(
						v.GetPoolDenoms(),
						"ibc/CE5BFF1D9BADA03BB5CCA5F56939392A761B53A10FBD03B37506669C3218D3B2", // HASH
					)
				}
			},
		},
		{
			name: "Sort by pool ID descending",
			options: []domain.PoolsOption{
				domain.WithSort(
					&v1beta1.SortRequest{
						Fields: []*v1beta1.SortField{
							{Field: "id", Direction: v1beta1.SortDirection_DESCENDING},
						},
					},
				),
			},
			expectedLen: 1957,
			expectError: false,
			validateFunc: func(s *PoolsUsecaseTestSuite, pools []sqsdomain.PoolI) {
				for i := 1; i < len(pools); i++ {
					s.Require().True(pools[i-1].GetId() > pools[i].GetId())
				}
			},
		},
	}

	// Run each test case
	for _, tt := range tests {
		s.Run(tt.name, func() {
			pools, _, err := usecase.Pools.GetPools(tt.options...)

			// Check for expected errors
			if tt.expectError {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
			}

			// Check pool lengths
			if tt.expectedLen > 0 {
				s.Require().Len(pools, tt.expectedLen)
			} else if tt.minExpected > 0 {
				s.Require().GreaterOrEqual(len(pools), tt.minExpected)
			}

			// Run custom validations if provided
			if tt.validateFunc != nil {
				tt.validateFunc(s, pools)
			}
		})
	}
}

func (s *PoolsUsecaseTestSuite) TestSetPoolAPRAndFeeDataIfConfigured() {
	var (
		// Helper functions to modify the APR and fee data
		withIsAPRStale = func(aprData sqspassthroughdomain.PoolAPRDataStatusWrap) sqspassthroughdomain.PoolAPRDataStatusWrap {
			aprData.IsStale = true
			return aprData
		}
		withIsAPRError = func(aprData sqspassthroughdomain.PoolAPRDataStatusWrap) sqspassthroughdomain.PoolAPRDataStatusWrap {
			aprData.IsError = true
			return aprData
		}
		withIsFeeStale = func(feeData sqspassthroughdomain.PoolFeesDataStatusWrap) sqspassthroughdomain.PoolFeesDataStatusWrap {
			feeData.IsStale = true
			return feeData
		}
		withIsFeeError = func(feeData sqspassthroughdomain.PoolFeesDataStatusWrap) sqspassthroughdomain.PoolFeesDataStatusWrap {
			feeData.IsError = true
			return feeData
		}

		// Empty APR and fee data
		emptyAPRData = sqspassthroughdomain.PoolAPRDataStatusWrap{}
		emptyFeeData = sqspassthroughdomain.PoolFeesDataStatusWrap{}
	)

	testCases := []struct {
		name string

		pool sqsdomain.PoolI
		opts []domain.PoolsOption

		shouldForceAPRFetcherError  bool
		shouldForceFeesFetcherError bool

		isAPRDataStale bool
		isFeeDataStale bool

		expectedAPRData  sqspassthroughdomain.PoolAPRDataStatusWrap
		expectedFeesData sqspassthroughdomain.PoolFeesDataStatusWrap
	}{
		{
			name: "no APR or fees data configured",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID,
			},

			opts: []domain.PoolsOption{},

			expectedAPRData:  emptyAPRData,
			expectedFeesData: emptyFeeData,
		},
		{
			name: "APR and fees data configured",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID,
			},

			opts: []domain.PoolsOption{
				domain.WithMarketIncentives(true),
			},

			expectedAPRData:  defaultAPRData,
			expectedFeesData: defaultFeeData,
		},
		{
			name: "APR and fees not confgiured due to different pool",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID + 1,
			},

			opts: []domain.PoolsOption{
				domain.WithMarketIncentives(true),
			},

			expectedAPRData:  emptyAPRData,
			expectedFeesData: emptyFeeData,
		},
		{
			name: "with apr and fee data both stale",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID,
			},

			opts: []domain.PoolsOption{
				domain.WithMarketIncentives(true),
			},

			shouldForceAPRFetcherError:  true,
			shouldForceFeesFetcherError: true,

			isAPRDataStale: true,
			isFeeDataStale: true,

			expectedAPRData:  withIsAPRError(withIsAPRStale(defaultAPRData)),
			expectedFeesData: withIsFeeError(withIsFeeStale(defaultFeeData)),
		},
	}

	for _, tc := range testCases {
		tc := tc
		s.Run(tc.name, func() {
			poolsUseCase := s.newDefaultPoolsUseCase()

			// Register mock APR fetcher
			mockAPRFetcher := getMockAPRFetcher(tc.shouldForceAPRFetcherError, tc.isAPRDataStale)
			poolsUseCase.RegisterAPRFetcher(mockAPRFetcher)

			// Register mock fees fetcher
			mockFeesFetcher := getMockFeesFetcher(tc.shouldForceFeesFetcherError, tc.isFeeDataStale)
			poolsUseCase.RegisterPoolFeesFetcher(mockFeesFetcher)

			// System under test
			var opts domain.PoolsOptions
			for _, opt := range tc.opts {
				opt(&opts)
			}
			poolsUseCase.SetPoolAPRAndFeeDataIfConfigured(tc.pool, opts)

			// Validate mutations
			s.Require().Equal(tc.expectedAPRData, tc.pool.GetAPRData())
			s.Require().Equal(tc.expectedFeesData, tc.pool.GetFeesData())
		})
	}
}

func (s *PoolsUsecaseTestSuite) newRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec) domain.RoutablePool {
	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(pool, tokenOutDenom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)
	return routablePool
}

func (s *PoolsUsecaseTestSuite) TestetPoolAPRAndFeeDataIfConfigured() {

}

func (s *PoolsUsecaseTestSuite) newDefaultPoolsUseCase() *usecase.PoolsUsecase {
	routerRepo := routerrepo.New(&log.NoOpLogger{})
	poolsUsecase, err := usecase.NewPoolsUsecase(&domain.PoolsConfig{}, "node-uri-placeholder", routerRepo, domain.UnsetScalingFactorGetterCb, nil, &log.NoOpLogger{})
	s.Require().NoError(err)
	return poolsUsecase
}

// Returns a mock APR fetcher that can be used to test the APR data fetching logic.
func getMockAPRFetcher(shouldForceAPRFetcherError, isAPRDataStale bool) *mocks.MapFetcherMock[uint64, sqspassthroughdomain.PoolAPR] {
	return &mocks.MapFetcherMock[uint64, sqspassthroughdomain.PoolAPR]{
		// Mock GetByKey
		GetByKeyFn: func(key uint64) (sqspassthroughdomain.PoolAPR, time.Time, bool, error) {
			var err error
			if shouldForceAPRFetcherError {
				err = defaultError
			}

			if key != defaultPoolID {
				return sqspassthroughdomain.PoolAPR{}, defaultTime, isAPRDataStale, err
			}

			return defaultAPRData.PoolAPR, defaultTime, isAPRDataStale, err
		},
	}
}

// Returns a mock fees fetcher that can be used to test the fees data fetching logic.
func getMockFeesFetcher(shouldForceFeesFetcherError, isFeeDataStale bool) *mocks.MapFetcherMock[uint64, sqspassthroughdomain.PoolFee] {
	return &mocks.MapFetcherMock[uint64, sqspassthroughdomain.PoolFee]{
		// Mock GetByKey
		GetByKeyFn: func(key uint64) (sqspassthroughdomain.PoolFee, time.Time, bool, error) {
			var err error
			if shouldForceFeesFetcherError {
				err = defaultError
			}

			if key != defaultPoolID {
				return sqspassthroughdomain.PoolFee{}, defaultTime, isFeeDataStale, err
			}

			return defaultFeeData.PoolFee, defaultTime, isFeeDataStale, err
		},
	}
}
