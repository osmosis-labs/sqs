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

var (
	UOSMO = routertesting.UOSMO
	USDC  = routertesting.USDC
)

type IngestUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

func TestIngestUseCaseTestSuite(t *testing.T) {
	suite.Run(t, new(IngestUseCaseTestSuite))
}

// Validates updateCurrentBlockLiquidityMapFromBalances per the spec.
func (s *IngestUseCaseTestSuite) TestUpdateCurrentBlockLiquidityMapFromBalances() {
	const (
		defaltPoolID uint64 = 1
	)

	var (
		emptyBlockLiqMap = domain.DenomLiquidityMap{}
		defaultAmount    = osmomath.NewInt(1_000)

		defaultUOSMOBalance = sdk.NewCoin(UOSMO, defaultAmount)

		defaultUSDCBalance = sdk.NewCoin(USDC, defaultAmount)

		defaultATOMBalance = sdk.NewCoin(routertesting.ATOM, defaultAmount)

		defaultMapUOSMOEntry = domain.DenomLiquidityMap{
			UOSMO: domain.DenomLiquidityData{
				TotalLiquidity: defaultAmount,
				Pools: map[uint64]osmomath.Int{
					defaltPoolID: defaultAmount,
				},
			},
		}
	)

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
			// TODO: consider error since this should not be possible.
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

			expectedBlockLiqMap: domain.DenomLiquidityMap{
				UOSMO: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
					},
				},

				USDC: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID + 1: defaultAmount,
					},
				},
			},
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

			blockLiqMap: domain.DenomLiquidityMap{},

			balances: sdk.NewCoins(),
			poolID:   defaltPoolID,

			expectedBlockLiqMap: domain.DenomLiquidityMap{},
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
					TotalLiquidity: defaultAmount.Add(defaultAmount),
					Pools: map[uint64]osmomath.Int{
						defaltPoolID: defaultAmount,
						// Another pool entry created.
						defaltPoolID + 1: defaultAmount,
					},
				},

				// New entry for atom created.
				routertesting.ATOM: domain.DenomLiquidityData{
					TotalLiquidity: defaultAmount,
					Pools: map[uint64]osmomath.Int{
						defaltPoolID + 1: defaultAmount,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		tc := tc

		s.T().Run(tc.name, func(t *testing.T) {

			// System under test.
			actualBlockLiqMap := usecase.UpdateCurrentBlockLiquidityMapFromBalances(tc.blockLiqMap, tc.balances, tc.poolID)

			// Validate.
			s.Require().Equal(tc.expectedBlockLiqMap, actualBlockLiqMap)
		})
	}
}
