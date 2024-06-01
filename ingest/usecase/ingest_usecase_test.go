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
	defaltPoolID uint64 = 1
)

var (
	emptyBlockLiqMap = domain.DenomLiquidityMap{}
	defaultAmount    = osmomath.NewInt(1_000)

	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
	ATOM  = routertesting.ATOM

	defaultUOSMOBalance = sdk.NewCoin(UOSMO, defaultAmount)

	defaultUSDCBalance = sdk.NewCoin(USDC, defaultAmount.Add(defaultAmount))

	defaultATOMBalance = sdk.NewCoin(ATOM, defaultAmount)

	defaultMapUOSMOEntry = domain.DenomLiquidityMap{
		UOSMO: domain.DenomLiquidityData{
			TotalLiquidity: defaultAmount,
			Pools: map[uint64]osmomath.Int{
				defaltPoolID: defaultAmount,
			},
		},
	}

	defaultUSDCLiquidityMapEntry = domain.DenomLiquidityMap{
		USDC: domain.DenomLiquidityData{
			TotalLiquidity: defaultAmount.Add(defaultAmount),
			Pools: map[uint64]osmomath.Int{
				defaltPoolID + 1: defaultAmount.Add(defaultAmount),
			},
		},
	}

	mergedUOSMOandUSDCMap = domain.DenomLiquidityMap{
		UOSMO: domain.DenomLiquidityData{
			TotalLiquidity: defaultAmount,
			Pools: map[uint64]osmomath.Int{
				defaltPoolID: defaultAmount,
			},
		},

		USDC: domain.DenomLiquidityData{
			TotalLiquidity: defaultUSDCBalance.Amount,
			Pools: map[uint64]osmomath.Int{
				defaltPoolID + 1: defaultUSDCBalance.Amount,
			},
		},
	}

	emptyDenomLiquidityMap = domain.DenomLiquidityMap{}
)

func TestIngestUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(IngestUseCaseTestSuite))
}

// Validates updateCurrentBlockLiquidityMapFromBalances per the spec.
func (s *IngestUseCaseTestSuite) TestUpdateCurrentBlockLiquidityMapFromBalances() {
	tests := []struct {
		name string

		blockLiqMap domain.DenomLiquidityMap
		balances    sdk.Coins
		poolID      uint64

		expectedBlockLiqMap domain.DenomLiquidityMap
	}{
		{
			name: "Empty map, empty balance, pool ID",

			blockLiqMap: emptyBlockLiqMap,

			poolID: defaltPoolID,

			expectedBlockLiqMap: emptyBlockLiqMap,
		},
		{
			name: "Token in map, one token in balance no pools pre-existing -> updated the token in map & added pool",

			blockLiqMap: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools:          map[uint64]osmomath.Int{},
				},
			},

			balances: sdk.NewCoins(defaultUOSMOBalance),

			poolID: defaltPoolID,

			expectedBlockLiqMap: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					// 2x original amount
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						// Pool entry added
						defaltPoolID: defaultAmount,
					},
				},
			},
		},
		{
			name: "One token in map, unrelated token in balance -> created separate entry",

			blockLiqMap: defaultMapUOSMOEntry,

			balances: sdk.NewCoins(defaultUSDCBalance),

			poolID: defaltPoolID + 1,

			expectedBlockLiqMap: mergedUOSMOandUSDCMap,
		},
		{
			name: "One token in map, zero tokens in balance -> no-op",

			blockLiqMap: defaultMapUOSMOEntry,

			balances: sdk.NewCoins(),
			poolID:   defaltPoolID,

			expectedBlockLiqMap: defaultMapUOSMOEntry,
		},

		{
			name: "Zero tokens in balance, none in map -> no-op",

			blockLiqMap: emptyDenomLiquidityMap,

			balances: sdk.NewCoins(),
			poolID:   defaltPoolID,

			expectedBlockLiqMap: emptyDenomLiquidityMap,
		},
		{
			name: "Some tokens in map, some tokens in balance -> updates as expected",

			blockLiqMap: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
					},
				},
				USDC: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
					},
				},
			},

			balances: sdk.NewCoins(
				defaultUOSMOBalance,
				defaultUSDCBalance,
				defaultATOMBalance,
			),
			poolID: defaltPoolID + 1,

			expectedBlockLiqMap: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					// Doubled
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
						// Another pool entry created.
						defaltPoolID + 1: defaultAmount,
					},
				},
				USDC: domain.DenomLiquidityData{
					// Doubled
					TotalLiquidity: defaultAmount.Add(defaultUSDCBalance.Amount),
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
						// Another pool entry created.
						defaltPoolID + 1: defaultUSDCBalance.Amount,
					},
				},

				// New entry for atom created.
				ATOM: domain.DenomLiquidityData{
					TotalLiquidity: defaultATOMBalance.Amount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID + 1: defaultATOMBalance.Amount,
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

		transferTo   domain.DenomLiquidityMap
		transferFrom domain.DenomLiquidityMap

		expectedResult domain.DenomLiquidityMap
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
			transferFrom: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaltPoolID:     defaultAmount,
						defaltPoolID + 1: defaultAmount,
					},
				},
			},

			expectedResult: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaltPoolID:     defaultAmount,
						defaltPoolID + 1: defaultAmount,
					},
				},
			},
		},

		{
			name: "2 entries in transfer from, 3 exist in transfer to (1 copied, 1 updated, 1 untouched)",

			transferTo: mergedUOSMOandUSDCMap,
			transferFrom: domain.DenomLiquidityMap{
				UOSMO: defaultUSDCLiquidityMapEntry[UOSMO],
				USDC:  defaultUSDCLiquidityMapEntry[USDC],
				ATOM: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
					},
				},
			},

			expectedResult: domain.DenomLiquidityMap{
				// Double UOSMO
				UOSMO: domain.DenomLiquidityData{
					TotalLiquidity: defaultUOSMOBalance.Amount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultUOSMOBalance.Amount,
					},
				},
				// Double USDC
				USDC: domain.DenomLiquidityData{
					TotalLiquidity: defaultUSDCBalance.Amount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID + 1: defaultUSDCBalance.Amount,
					},
				},

				// ATOM unchanged.
				ATOM: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
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
func deepCopyDenomLiquidityMap(m domain.DenomLiquidityMap) domain.DenomLiquidityMap {
	copy := make(domain.DenomLiquidityMap, len(m))
	for k, v := range m {
		copy[k] = domain.DenomLiquidityData{
			TotalLiquidity: v.TotalLiquidity,
			Pools:          make(map[uint64]osmomath.Int, len(v.Pools)),
		}
		for pk, pv := range v.Pools {
			copy[k].Pools[pk] = pv
		}
	}
	return copy
}
