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

	cosmwasmpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
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

	cosmWasmPoolsParams := pools.CosmWasmPoolsParams{
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

			// Create router repository
			routerRepo := routerrepo.New(&log.NoOpLogger{})
			routerRepo.SetTakerFees(tc.takerFeeMap)

			// Create pools use case
			poolsUsecase, err := usecase.NewPoolsUsecase(&domain.PoolsConfig{}, "node-uri-placeholder", routerRepo, domain.UnsetScalingFactorGetterCb, &log.NoOpLogger{})
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
				tokenIn := sdk.NewCoin(tc.tokenInDenom, sdk.NewInt(100))
				actualPools, _, _, err := actualRoute.PrepareResultPools(context.TODO(), tokenIn)
				s.Require().NoError(err)
				expectedPools, _, _, err := expectedRoute.PrepareResultPools(context.TODO(), tokenIn)
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

func (s *PoolsUsecaseTestSuite) TestGetPools() {
	mainnetState := s.SetupMainnetState()

	usecase := s.SetupRouterAndPoolsUsecase(mainnetState)

	// No filter
	pools, err := usecase.Pools.GetPools()
	s.Require().NoError(err)
	s.Require().True(len(pools) > 1500)

	// Pool 32 is garbage and has zero liq.
	// Pools 1 and 1066 are major pools.
	poolsFilter := []uint64{32, 1, 1066}

	// Pool ID filter
	pools, err = usecase.Pools.GetPools(domain.WithPoolIDFilter(poolsFilter))
	s.Require().NoError(err)
	s.Require().Len(pools, len(poolsFilter))

	// Min liquidity cap filter
	pools, err = usecase.Pools.GetPools(domain.WithMinPoolsLiquidityCap(1_000_000))
	s.Require().NoError(err)
	s.Require().True(len(pools) < 100)

	pools, err = usecase.Pools.GetPools(domain.WithMinPoolsLiquidityCap(1), domain.WithPoolIDFilter(poolsFilter))
	s.Require().NoError(err)
	s.Require().Len(pools, 2)

	// Empty filter signifies returning nothing and exiting early
	pools, err = usecase.Pools.GetPools(domain.WithPoolIDFilter([]uint64{}))
	s.Require().NoError(err)
	s.Require().Empty(pools)
}

func (s *PoolsUsecaseTestSuite) TestSetPoolAPRAndFeeDataIfConfigured() {
	var (
		// Helper functions to modify the APR and fee data
		withIsAPRStale = func(aprData passthroughdomain.PoolAPRDataStatusWrap) passthroughdomain.PoolAPRDataStatusWrap {
			aprData.IsStale = true
			return aprData
		}
		withIsAPRError = func(aprData passthroughdomain.PoolAPRDataStatusWrap) passthroughdomain.PoolAPRDataStatusWrap {
			aprData.IsError = true
			return aprData
		}
		withIsFeeStale = func(feeData passthroughdomain.PoolFeesDataStatusWrap) passthroughdomain.PoolFeesDataStatusWrap {
			feeData.IsStale = true
			return feeData
		}
		withIsFeeError = func(feeData passthroughdomain.PoolFeesDataStatusWrap) passthroughdomain.PoolFeesDataStatusWrap {
			feeData.IsError = true
			return feeData
		}

		// Default APR and fee data
		defaultAPRData = passthroughdomain.PoolAPRDataStatusWrap{PoolAPR: passthroughdomain.PoolAPR{
			PoolID: defaultPoolID,
			SwapFees: passthroughdomain.PoolDataRange{
				Lower: 0.01,
				Upper: 0.02,
			},
		}}
		defaultFeeData = passthroughdomain.PoolFeesDataStatusWrap{
			PoolFee: passthroughdomain.PoolFee{
				PoolID: fmt.Sprintf("%d", defaultPoolID),
			},
		}

		// Default values
		defaultTime  = time.Unix(0, 0)
		defaultError = fmt.Errorf("forced error")

		// Empty APR and fee data
		emptyAPRData = passthroughdomain.PoolAPRDataStatusWrap{}
		emptyFeeData = passthroughdomain.PoolFeesDataStatusWrap{}
	)

	testCases := []struct {
		name string

		pool sqsdomain.PoolI
		opts domain.PoolsOptions

		shouldForceAPRFetcherError  bool
		shouldForceFeesFetcherError bool

		isAPRDataStale bool
		isFeeDataStale bool

		expectedAPRData  passthroughdomain.PoolAPRDataStatusWrap
		expectedFeesData passthroughdomain.PoolFeesDataStatusWrap
	}{
		{
			name: "no APR or fees data configured",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID,
			},

			opts: domain.PoolsOptions{},

			expectedAPRData:  emptyAPRData,
			expectedFeesData: emptyFeeData,
		},
		{
			name: "APR and fees data configured",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID,
			},

			opts: domain.PoolsOptions{
				WithMarketIncentives: true,
			},

			expectedAPRData:  defaultAPRData,
			expectedFeesData: defaultFeeData,
		},
		{
			name: "APR and fees not confgiured due to different pool",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID + 1,
			},

			opts: domain.PoolsOptions{
				WithMarketIncentives: true,
			},

			expectedAPRData:  emptyAPRData,
			expectedFeesData: emptyFeeData,
		},
		{
			name: "with apr and fee data both stale",

			pool: &mocks.MockRoutablePool{
				ID: defaultPoolID,
			},

			opts: domain.PoolsOptions{
				WithMarketIncentives: true,
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
			mockAPRFetcher := &mocks.MapFetcherMock[uint64, passthroughdomain.PoolAPR]{
				// Mock GetByKey
				GetByKeyFn: func(key uint64) (passthroughdomain.PoolAPR, time.Time, bool, error) {
					var err error
					if tc.shouldForceAPRFetcherError {
						err = defaultError
					}

					if key != defaultPoolID {
						return passthroughdomain.PoolAPR{}, defaultTime, tc.isAPRDataStale, err
					}

					return defaultAPRData.PoolAPR, defaultTime, tc.isAPRDataStale, err
				},
			}
			poolsUseCase.RegisterAPRFetcher(mockAPRFetcher)

			// Register mock fees fetcher
			mockFeesFetcher := &mocks.MapFetcherMock[uint64, passthroughdomain.PoolFee]{
				// Mock GetByKey
				GetByKeyFn: func(key uint64) (passthroughdomain.PoolFee, time.Time, bool, error) {
					var err error
					if tc.shouldForceFeesFetcherError {
						err = defaultError
					}

					if key != defaultPoolID {
						return passthroughdomain.PoolFee{}, defaultTime, tc.isFeeDataStale, err
					}

					return defaultFeeData.PoolFee, defaultTime, tc.isFeeDataStale, err
				},
			}
			poolsUseCase.RegisterPoolFeesFetcher(mockFeesFetcher)

			// System under test
			poolsUseCase.SetPoolAPRAndFeeDataIfConfigured(tc.pool, tc.opts)

			// Validate mutations
			s.Require().Equal(tc.expectedAPRData, tc.pool.GetAPRData())
			s.Require().Equal(tc.expectedFeesData, tc.pool.GetFeesData())
		})
	}
}

func (s *PoolsUsecaseTestSuite) newRoutablePool(pool sqsdomain.PoolI, tokenOutDenom string, takerFee osmomath.Dec) domain.RoutablePool {
	cosmWasmPoolsParams := pools.CosmWasmPoolsParams{
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
	poolsUsecase, err := usecase.NewPoolsUsecase(&domain.PoolsConfig{}, "node-uri-placeholder", routerRepo, domain.UnsetScalingFactorGetterCb, &log.NoOpLogger{})
	s.Require().NoError(err)
	return poolsUsecase
}
