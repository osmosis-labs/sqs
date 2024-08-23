package usecase_test

import (
	"context"
	"math/rand"
	"sync"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
	"github.com/stretchr/testify/suite"
)

type IngestUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

const (
	defaultPoolID uint64 = 1
)

var (
	emptyBlockLiqMap = domain.DenomPoolLiquidityMap{}
	defaultAmount    = osmomath.NewInt(1_000)

	UOSMO   = routertesting.UOSMO
	USDC    = routertesting.USDC
	ATOM    = routertesting.ATOM
	ALLBTC  = routertesting.ALLBTC
	ALLUSDT = routertesting.ALLUSDT

	defaultUOSMOBalance = sdk.NewCoin(UOSMO, defaultAmount)

	defaultUSDCBalance = sdk.NewCoin(USDC, defaultAmount.Add(defaultAmount))

	defaultATOMBalance = sdk.NewCoin(ATOM, defaultAmount)

	defaultMapUOSMOEntry = domain.DenomPoolLiquidityMap{
		UOSMO: domain.DenomPoolLiquidityData{
			TotalLiquidity: defaultAmount,
			Pools: map[uint64]osmomath.Int{
				defaultPoolID: defaultAmount,
			},
		},
	}

	defaultUSDCLiquidityMapEntry = domain.DenomPoolLiquidityMap{
		USDC: domain.DenomPoolLiquidityData{
			TotalLiquidity: defaultAmount.Add(defaultAmount),
			Pools: map[uint64]osmomath.Int{
				defaultPoolID + 1: defaultAmount.Add(defaultAmount),
			},
		},
	}

	mergedUOSMOandUSDCMap = domain.DenomPoolLiquidityMap{
		UOSMO: domain.DenomPoolLiquidityData{
			TotalLiquidity: defaultAmount,
			Pools: map[uint64]osmomath.Int{
				defaultPoolID: defaultAmount,
			},
		},

		USDC: domain.DenomPoolLiquidityData{
			TotalLiquidity: defaultUSDCBalance.Amount,
			Pools: map[uint64]osmomath.Int{
				defaultPoolID + 1: defaultUSDCBalance.Amount,
			},
		},
	}

	emptyDenomLiquidityMap = domain.DenomPoolLiquidityMap{}

	zeroInt    = osmomath.ZeroInt()
	oneInt     = osmomath.OneInt()
	noOpLogger = &log.NoOpLogger{}
)

func TestIngestUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(IngestUseCaseTestSuite))
}

// Validates updateCurrentBlockLiquidityMapFromBalances per the spec.
func (s *IngestUseCaseTestSuite) TestUpdateCurrentBlockLiquidityMapFromBalances() {
	tests := []struct {
		name string

		blockLiqMap domain.DenomPoolLiquidityMap
		balances    sdk.Coins
		poolID      uint64

		expectedBlockLiqMap domain.DenomPoolLiquidityMap
	}{
		{
			name: "Empty map, empty balance, pool ID",

			blockLiqMap: emptyBlockLiqMap,

			poolID: defaultPoolID,

			expectedBlockLiqMap: emptyBlockLiqMap,
		},
		{
			name: "Token in map, one token in balance no pools pre-existing -> updated the token in map & added pool",

			blockLiqMap: domain.DenomPoolLiquidityMap{
				UOSMO: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools:          map[uint64]osmomath.Int{},
				},
			},

			balances: sdk.NewCoins(defaultUOSMOBalance),

			poolID: defaultPoolID,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{
				UOSMO: domain.DenomPoolLiquidityData{
					// 2x original amount
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						// Pool entry added
						defaultPoolID: defaultAmount,
					},
				},
			},
		},
		{
			name: "One token in map, unrelated token in balance -> created separate entry",

			blockLiqMap: defaultMapUOSMOEntry,

			balances: sdk.NewCoins(defaultUSDCBalance),

			poolID: defaultPoolID + 1,

			expectedBlockLiqMap: mergedUOSMOandUSDCMap,
		},
		{
			name: "One token in map, zero tokens in balance -> no-op",

			blockLiqMap: defaultMapUOSMOEntry,

			balances: sdk.NewCoins(),
			poolID:   defaultPoolID,

			expectedBlockLiqMap: defaultMapUOSMOEntry,
		},

		{
			name: "Zero tokens in balance, none in map -> no-op",

			blockLiqMap: emptyDenomLiquidityMap,

			balances: sdk.NewCoins(),
			poolID:   defaultPoolID,

			expectedBlockLiqMap: emptyDenomLiquidityMap,
		},
		{
			name: "Some tokens in map, some tokens in balance -> updates as expected",

			blockLiqMap: domain.DenomPoolLiquidityMap{
				UOSMO: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultAmount,
					},
				},
				USDC: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultAmount,
					},
				},
			},

			balances: sdk.NewCoins(
				defaultUOSMOBalance,
				defaultUSDCBalance,
				defaultATOMBalance,
			),
			poolID: defaultPoolID + 1,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{
				UOSMO: domain.DenomPoolLiquidityData{
					// Doubled
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultAmount,
						// Another pool entry created.
						defaultPoolID + 1: defaultAmount,
					},
				},
				USDC: domain.DenomPoolLiquidityData{
					// Doubled
					TotalLiquidity: defaultAmount.Add(defaultUSDCBalance.Amount),
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultAmount,
						// Another pool entry created.
						defaultPoolID + 1: defaultUSDCBalance.Amount,
					},
				},

				// New entry for atom created.
				ATOM: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultATOMBalance.Amount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID + 1: defaultATOMBalance.Amount,
					},
				},
			},
		},
		{
			name: "Invalid token in balance -> skipped",

			blockLiqMap: domain.DenomPoolLiquidityMap{},

			balances: sdk.Coins{sdk.Coin{Denom: "[[]invalid[]]", Amount: osmomath.OneInt()}},

			poolID: defaultPoolID + 1,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{},
		},
	}

	for _, tc := range tests {
		tc := tc
		s.T().Run(tc.name, func(t *testing.T) {
			// Note that the transferTo parameter is mutated, so we need to copy it
			// to avoid flakiness across tests.
			blockLiqMapCopy := deepCopyDenomLiquidityMap(tc.blockLiqMap)

			// System under test.
			actualBlockLiqMap := usecase.UpdateCurrentBlockLiquidityMapFromBalances(blockLiqMapCopy, tc.balances, tc.poolID)

			// Validate.
			s.Require().Equal(tc.expectedBlockLiqMap, actualBlockLiqMap)
		})
	}
}

// Validates transferDenomLiquidityMap per the spec.
func (s *IngestUseCaseTestSuite) TestTransferDenomLiquidityMap() {
	tests := []struct {
		name string

		transferTo   domain.DenomPoolLiquidityMap
		transferFrom domain.DenomPoolLiquidityMap

		expectedResult domain.DenomPoolLiquidityMap
	}{
		{
			name: "both empty -> no-op",

			transferTo:   emptyDenomLiquidityMap,
			transferFrom: emptyDenomLiquidityMap,

			expectedResult: emptyDenomLiquidityMap,
		},

		{
			name: "transferTo empty -> transferred over",

			transferTo:   emptyDenomLiquidityMap,
			transferFrom: defaultMapUOSMOEntry,

			expectedResult: defaultMapUOSMOEntry,
		},

		{
			name: "transferFrom empty -> no-op",

			transferTo:   defaultMapUOSMOEntry,
			transferFrom: emptyBlockLiqMap,

			expectedResult: defaultMapUOSMOEntry,
		},

		{
			name: "entry is in transferFrom but not transferTo -> copied over",

			transferTo:   defaultMapUOSMOEntry,
			transferFrom: defaultUSDCLiquidityMapEntry,

			expectedResult: mergedUOSMOandUSDCMap,
		},

		{
			name: "same entry is in transferTo and transferFrom -> overwritten",

			transferTo: defaultMapUOSMOEntry,
			transferFrom: domain.DenomPoolLiquidityMap{
				UOSMO: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaultPoolID:     defaultAmount,
						defaultPoolID + 1: defaultAmount,
					},
				},
			},

			expectedResult: domain.DenomPoolLiquidityMap{
				UOSMO: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaultPoolID:     defaultAmount,
						defaultPoolID + 1: defaultAmount,
					},
				},
			},
		},

		{
			name: "2 entries in transfer from, 3 exist in transfer to (1 copied, 1 updated, 1 untouched)",

			transferTo: mergedUOSMOandUSDCMap,
			transferFrom: domain.DenomPoolLiquidityMap{
				UOSMO: defaultUSDCLiquidityMapEntry[UOSMO],
				USDC:  defaultUSDCLiquidityMapEntry[USDC],
				ATOM: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultAmount,
					},
				},
			},

			expectedResult: domain.DenomPoolLiquidityMap{
				// Double UOSMO
				UOSMO: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultUOSMOBalance.Amount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultUOSMOBalance.Amount,
					},
				},
				// Double USDC
				USDC: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultUSDCBalance.Amount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID + 1: defaultUSDCBalance.Amount,
					},
				},

				// ATOM unchanged.
				ATOM: domain.DenomPoolLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: defaultAmount,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		s.T().Run(tc.name, func(t *testing.T) {
			// Note that the transferTo parameter is mutated, so we need to copy it
			// to avoid flakiness across tests.
			transferToCopy := deepCopyDenomLiquidityMap(tc.transferTo)

			// System under test
			result := usecase.TransferDenomLiquidityMap(transferToCopy, tc.transferFrom)

			// Validation
			s.Require().Equal(tc.expectedResult, result)
		})
	}
}

func (s *IngestUseCaseTestSuite) TestCallUpdateAssetsAtHeightIntervalSync() {
	tests := []struct {
		name          string
		wantCallCount int
	}{
		{
			name:          "Random call count",
			wantCallCount: rand.Intn(100),
		},
		{
			name:          "Zero call count",
			wantCallCount: 0,
		},
		{
			name:          "Fixed call count",
			wantCallCount: 50,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var (
				// got is used to count the number of times UpdateAssetsAtHeightIntervalSync
				// UpdateAssetsAtHeightIntervalSync is being called as a separate goroutine
				// inside the ProcessBlockData function.
				got int

				// mu is used to synchronize access to got.
				mu sync.Mutex

				// wg is used to wait for all UpdateAssetsAtHeightIntervalSync goroutines to finish.
				wg sync.WaitGroup
			)

			// Add the number of expected calls to the wait group.
			wg.Add(tt.wantCallCount)

			ingester, err := usecase.NewIngestUsecase(
				&mocks.PoolsUsecaseMock{
					StorePoolsFunc: func(pools []sqsdomain.PoolI) error {
						return nil
					},
				},
				&mocks.RouterUsecaseMock{},
				&mocks.RouterUsecaseMock{},
				&mocks.TokensUsecaseMock{
					UpdateAssetsAtHeightIntervalSyncFunc: func(height uint64) error {
						defer wg.Done()
						mu.Lock()
						got++
						mu.Unlock()
						return nil
					},
				},
				&mocks.ChainInfoUsecaseMock{},
				nil,
				&mocks.PricingWorkerMock{
					UpdatePricesAsyncFunc: func(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
						// do nothing
					},
				},
				&mocks.CandidateRouteSearchDataWorkerMock{},
				nil,
				noOpLogger,
			)
			s.Require().NoError(err)

			for height := 0; height < tt.wantCallCount; height++ {
				err = ingester.ProcessBlockData(context.TODO(), uint64(height)+1, nil, nil)
				s.Require().NoError(err)
			}

			wg.Wait()

			s.Require().Equal(tt.wantCallCount, got)
		})
	}
}

func (s *IngestUseCaseTestSuite) TestProcessSQSModelMut() {

	var (
		defaultModel = &sqsdomain.SQSPool{
			PoolLiquidityCap:      osmomath.NewInt(1_000),
			PoolLiquidityCapError: "",
			Balances:              sdk.NewCoins(defaultUOSMOBalance, defaultUSDCBalance),
			PoolDenoms:            []string{UOSMO, USDC},
			SpreadFactor:          osmomath.NewDec(1),
			CosmWasmPoolModel:     nil,
		}

		// We reorder them by balances value
		reorderedDefaultDenoms = []string{USDC, UOSMO}

		deepCopy = func(sqsPool *sqsdomain.SQSPool) *sqsdomain.SQSPool {
			copy := *sqsPool

			copy.PoolLiquidityCap = sqsPool.PoolLiquidityCap.ToLegacyDec().TruncateInt()
			copy.PoolLiquidityCapError = sqsPool.PoolLiquidityCapError
			copy.Balances = sdk.NewCoins(sqsPool.Balances...)
			copy.PoolDenoms = append([]string(nil), sqsPool.PoolDenoms...)
			copy.SpreadFactor = sqsPool.SpreadFactor.Clone()

			// Not a deep copy because it is irrelevant for this test.
			copy.CosmWasmPoolModel = sqsPool.CosmWasmPoolModel

			return &copy
		}

		defaultCosmWasmModel = &cosmwasmpool.CosmWasmPoolModel{
			ContractInfo: cosmwasmpool.ContractInfo{
				Contract: cosmwasmpool.ALLOY_TRANSMUTER_CONTRACT_NAME,
				Version:  cosmwasmpool.ALLOY_TRANSMUTER_MIN_CONTRACT_VERSION,
			},
			Data: cosmwasmpool.CosmWasmPoolData{
				AlloyTransmuter: &cosmwasmpool.AlloyTransmuterData{
					AlloyedDenom: routertesting.ALLUSDT,
				},
			},
		}

		invalidCosmWasmModel = &cosmwasmpool.CosmWasmPoolModel{
			ContractInfo: cosmwasmpool.ContractInfo{
				Contract: cosmwasmpool.ALLOY_TRANSMUTER_CONTRACT_NAME,
				Version:  cosmwasmpool.ALLOY_TRANSMUTER_MIN_CONTRACT_VERSION,
			},

			// Note: data is missing
		}

		withCosmWasmModel = func(sqsPool *sqsdomain.SQSPool, cosmWasmModel *cosmwasmpool.CosmWasmPoolModel) *sqsdomain.SQSPool {
			sqsPool = deepCopy(sqsPool)
			sqsPool.CosmWasmPoolModel = cosmWasmModel
			return sqsPool
		}

		withPoolDenoms = func(sqsPool *sqsdomain.SQSPool, denoms ...string) *sqsdomain.SQSPool {
			sqsPool = deepCopy(sqsPool)
			sqsPool.PoolDenoms = denoms
			return sqsPool
		}

		withBalances = func(sqsPool *sqsdomain.SQSPool, balances sdk.Coins) *sqsdomain.SQSPool {
			sqsPool = deepCopy(sqsPool)
			sqsPool.Balances = balances
			return sqsPool
		}

		withAssetConfigs = func(sqsPool *sqsdomain.SQSPool, assetConfigs []cosmwasmpool.TransmuterAssetConfig) *sqsdomain.SQSPool {
			sqsPool = deepCopy(sqsPool)
			sqsPool.CosmWasmPoolModel.Data.AlloyTransmuter.AssetConfigs = assetConfigs
			return sqsPool
		}

		reorderedDefaultModel = withPoolDenoms(defaultModel, reorderedDefaultDenoms...)

		modelWithCWModelSet = withCosmWasmModel(defaultModel, defaultCosmWasmModel)
	)

	tests := []struct {
		name string

		sqsModel *sqsdomain.SQSPool

		expectedSQSModel *sqsdomain.SQSPool
		expectedErr      bool
	}{
		{
			name: "non-cosmwaspool model -> unchanged",

			sqsModel: defaultModel,

			expectedSQSModel: reorderedDefaultModel,
		},
		{
			name: "with gamm share in balance -> filtered",

			sqsModel: withBalances(defaultModel, sdk.NewCoins(sdk.NewCoin(domain.GAMMSharePrefix, osmomath.OneInt())).Add(defaultModel.Balances...)),

			expectedSQSModel: reorderedDefaultModel,
		},
		{
			name: "with gamm share in pool denoms -> filtered",

			// Note: append wrangling is done to avoid mutation of defaultModel.
			sqsModel: withPoolDenoms(defaultModel, append([]string{domain.GAMMSharePrefix}, defaultModel.PoolDenoms...)...),

			expectedSQSModel: defaultModel,
		},
		{
			name: "alloyed cosmwasm model -> added to pool denoms",

			sqsModel: modelWithCWModelSet,

			// Note: append wrangling is done to avoid mutation of defaultModel.
			expectedSQSModel: withAssetConfigs(withPoolDenoms(modelWithCWModelSet, append(append([]string{}, reorderedDefaultDenoms...), routertesting.ALLUSDT)...), []cosmwasmpool.TransmuterAssetConfig{
				{
					Denom:               ATOM,
					NormalizationFactor: tenE6,
				},
			}),
		},
		{
			name: "cosmwasm model not correctly set -> error",

			sqsModel: withPoolDenoms(withCosmWasmModel(defaultModel, invalidCosmWasmModel), append(reorderedDefaultDenoms, routertesting.ALLUSDT)...),

			expectedErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		s.T().Run(tc.name, func(t *testing.T) {
			// System under test
			err := usecase.ProcessSQSModelMut(tc.sqsModel)

			if tc.expectedErr {
				s.Require().Error(err)
			} else {
				s.Require().NoError(err)
				s.Require().Equal(tc.expectedSQSModel, tc.sqsModel)
			}
		})
	}
}

func (s *IngestUseCaseTestSuite) TestUpdateCurrentBlockLiquidityMapAlloyed() {
	var (
		nonEmptyEntry = domain.DenomPoolLiquidityData{
			TotalLiquidity: oneInt,
			Pools: map[uint64]osmomath.Int{
				defaultPoolID: oneInt,
			},
		}
		nonEmptyAllBTCMap = domain.DenomPoolLiquidityMap{
			ALLBTC: nonEmptyEntry,
		}
	)

	tests := []struct {
		name string

		blockLiqMap  domain.DenomPoolLiquidityMap
		balances     sdk.Coins
		poolID       uint64
		alloyedDenom string

		expectedBlockLiqMap domain.DenomPoolLiquidityMap
	}{
		{
			name: "Empty map, pool ID -> initializes to zero for the alloyed denom",

			blockLiqMap: emptyBlockLiqMap,

			poolID: defaultPoolID,

			alloyedDenom: ALLBTC,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{
				ALLBTC: domain.DenomPoolLiquidityData{
					TotalLiquidity: zeroInt,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: zeroInt,
					},
				},
			},
		},
		{
			name: "Non-empty map with different pool id -> initializes to zero for the alloyed denom but does not affect other pool id",

			blockLiqMap: nonEmptyAllBTCMap,

			poolID: defaultPoolID + 1,

			alloyedDenom: ALLBTC,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{
				ALLBTC: domain.DenomPoolLiquidityData{
					TotalLiquidity: oneInt,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID:     oneInt,
						defaultPoolID + 1: zeroInt,
					},
				},
			},
		},
		{
			name: "Non-empty map with same pool id -> initializes to zero, overriding the previous entry but does not affect total",

			blockLiqMap: nonEmptyAllBTCMap,

			poolID: defaultPoolID,

			alloyedDenom: ALLBTC,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{
				ALLBTC: domain.DenomPoolLiquidityData{
					// Note: this does not affect the previous liquidity.
					// But we expect for this case to never happen.
					TotalLiquidity: oneInt,
					Pools: map[uint64]osmomath.Int{
						// Note: this simply overwrites the previous entry.
						defaultPoolID: zeroInt,
					},
				},
			},
		},
		{
			name: "Non-empty map with same pool id but different denom",

			blockLiqMap: nonEmptyAllBTCMap,

			poolID: defaultPoolID,

			alloyedDenom: ALLUSDT,

			expectedBlockLiqMap: domain.DenomPoolLiquidityMap{
				ALLBTC: nonEmptyEntry,
				ALLUSDT: domain.DenomPoolLiquidityData{
					TotalLiquidity: zeroInt,
					Pools: map[uint64]osmomath.Int{
						defaultPoolID: zeroInt,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		s.T().Run(tc.name, func(t *testing.T) {
			// Note that the transferTo parameter is mutated, so we need to copy it
			// to avoid flakiness across tests.
			blockLiqMapCopy := deepCopyDenomLiquidityMap(tc.blockLiqMap)

			// System under test.
			actualBlockLiqMap := usecase.UpdateCurrentBlockLiquidityMapAlloyed(blockLiqMapCopy, tc.poolID, tc.alloyedDenom)

			// Validate.
			s.Require().Equal(tc.expectedBlockLiqMap, actualBlockLiqMap)
		})
	}
}

// deepCopyDenomLiquidityMap is a helper function to deep copy a DenomLiquidityMap.
func deepCopyDenomLiquidityMap(m domain.DenomPoolLiquidityMap) domain.DenomPoolLiquidityMap {
	copy := make(domain.DenomPoolLiquidityMap, len(m))
	for k, v := range m {
		copy[k] = domain.DenomPoolLiquidityData{
			TotalLiquidity: v.TotalLiquidity,
			Pools:          make(map[uint64]osmomath.Int, len(v.Pools)),
		}
		for pk, pv := range v.Pools {
			copy[k].Pools[pk] = pv
		}
	}
	return copy
}
