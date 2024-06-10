package usecase_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
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

	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
	ATOM  = routertesting.ATOM

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
