package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"
	cosmwasmpool "github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain/mocks"
)

type RouterTestSuite struct {
	routertesting.RouterTestHelper
}

func TestRouterTestSuite(t *testing.T) {
	suite.Run(t, new(RouterTestSuite))
}

const (
	dummyPoolLiquidityCapErrorStr = "pool liquidity cap error string"

	// OSMO token precision
	OsmoPrecisionMultiplier = 1000000
)

var (
	// Concentrated liquidity constants
	Denom0 = ETH
	Denom1 = USDC

	DefaultCurrentTick = routertesting.DefaultCurrentTick

	DefaultAmt0 = routertesting.DefaultAmt0
	DefaultAmt1 = routertesting.DefaultAmt1

	DefaultCoin0 = routertesting.DefaultCoin0
	DefaultCoin1 = routertesting.DefaultCoin1

	DefaultLiquidityAmt = routertesting.DefaultLiquidityAmt

	// router specific variables
	defaultTickModel = &sqsdomain.TickModel{
		Ticks:            []sqsdomain.LiquidityDepthsWithRange{},
		CurrentTickIndex: 0,
		HasNoLiquidity:   false,
	}

	noTakerFee = osmomath.ZeroDec()

	emptyCosmWasmPoolsRouterConfig = routertesting.EmpyCosmWasmPoolRouterConfig
)

// This test validates a happy path expected behavior that
// when router is created, it first takes the preferred pool IDs,
// then sorts by TVL.
// Other configurations parameters are also validated.
func (s *RouterTestSuite) TestRouterSorting() {
	s.Setup()

	// Prepare all supported pools.
	allPool := s.PrepareAllSupportedPoolsCustomProject("sqs", "scripts")

	// Create additional pools for edge cases
	var (
		secondBalancerPoolPoolID = s.PrepareBalancerPool()
		thirdBalancerPoolID      = s.PrepareBalancerPool()

		alloyedPoolID = thirdBalancerPoolID + 1

		// Note that these default denoms might not actually match the pool denoms for simplicity.
		defaultDenoms = []string{"foo", "bar"}
	)

	// Get balancer pool
	balancerPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, allPool.BalancerPoolID)
	s.Require().NoError(err)

	// Get stableswap pool
	stableswapPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, allPool.StableSwapPoolID)
	s.Require().NoError(err)

	// Get CL pool
	concentratedPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, allPool.ConcentratedPoolID)
	s.Require().NoError(err)

	// Get CosmWasm pool
	cosmWasmPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, allPool.CosmWasmPoolID)
	s.Require().NoError(err)

	// Get second & third balancer pools
	// Note that his pool is preferred but has TVL error flag set.
	secondBalancerPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, secondBalancerPoolPoolID)
	s.Require().NoError(err)

	// Note that his pool is not preferred and has TVL error flag set.
	thirdBalancerPool, err := s.App.PoolManagerKeeper.GetPool(s.Ctx, thirdBalancerPoolID)
	s.Require().NoError(err)

	var (
		// Inputs
		minPoolLiquidityCap = 2
		logger, _           = log.NewLogger(false, "", "")
		defaultAllPools     = []sqsdomain.PoolI{
			&sqsdomain.PoolWrapper{
				ChainModel: balancerPool,
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap: osmomath.NewInt(5 * OsmoPrecisionMultiplier), // 5
					PoolDenoms:       defaultDenoms,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: stableswapPool,
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap: osmomath.NewInt(int64(minPoolLiquidityCap) - 1), // 1
					PoolDenoms:       defaultDenoms,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: concentratedPool,
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap: osmomath.NewInt(4 * OsmoPrecisionMultiplier), // 4
					PoolDenoms:       defaultDenoms,
				},
				TickModel: &sqsdomain.TickModel{
					Ticks: []sqsdomain.LiquidityDepthsWithRange{
						{
							LowerTick:       0,
							UpperTick:       100,
							LiquidityAmount: osmomath.NewDec(100),
						},
					},
					CurrentTickIndex: 0,
					HasNoLiquidity:   false,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: cosmWasmPool,
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap: osmomath.NewInt(3 * OsmoPrecisionMultiplier), // 3
					PoolDenoms:       defaultDenoms,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: &mocks.ChainPoolMock{ID: alloyedPoolID, Type: poolmanagertypes.CosmWasm},
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap: osmomath.NewInt(3*OsmoPrecisionMultiplier - 1), // 3 * precision - 1
					PoolDenoms:       defaultDenoms,
					CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
						ContractInfo: cosmwasmpool.ContractInfo{
							Contract: cosmwasmpool.AlloyTranmuterName,
							Version:  cosmwasmpool.AlloyTransmuterMinVersion,
						},
					},
				},
			},

			// Note that the pools below have higher TVL.
			// However, since they have TVL error flag set, they
			// should be sorted after other pools, unless overriden by preferredPoolIDs.
			&sqsdomain.PoolWrapper{
				ChainModel: secondBalancerPool,
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap:      osmomath.NewInt(10 * OsmoPrecisionMultiplier), // 10
					PoolDenoms:            defaultDenoms,
					PoolLiquidityCapError: dummyPoolLiquidityCapErrorStr,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: thirdBalancerPool,
				SQSModel: sqsdomain.SQSPool{
					PoolLiquidityCap:      osmomath.NewInt(11 * OsmoPrecisionMultiplier), // 11
					PoolDenoms:            defaultDenoms,
					PoolLiquidityCapError: dummyPoolLiquidityCapErrorStr,
				},
			},
		}

		// Expected
		// First, preferred pool IDs, then sorted by TVL.
		expectedSortedPoolIDs = []uint64{
			// Transmuter pool is first due to no slippage swaps
			allPool.CosmWasmPoolID,

			// Alloyed is second since it has the same bonus as transmuter but lower
			// liquidity cap.
			alloyedPoolID,

			// Balancer is above concentrated pool due to being preferred
			allPool.BalancerPoolID,

			allPool.ConcentratedPoolID,

			thirdBalancerPoolID, // non-preferred pool ID with TVL error flag set

			secondBalancerPoolPoolID, // preferred pool ID with TVL error flag set

			allPool.StableSwapPoolID,
		}
	)

	cosmWasmPoolConfig := domain.CosmWasmPoolRouterConfig{
		TransmuterCodeIDs: map[uint64]struct{}{
			1: {},
		},
		GeneralCosmWasmCodeIDs:   map[uint64]struct{}{},
		ChainGRPCGatewayEndpoint: "",
	}

	totalTVL := osmomath.ZeroInt()
	for _, pool := range defaultAllPools {
		totalTVL = totalTVL.Add(pool.GetPoolLiquidityCap())
	}

	sortedPools := routerusecase.SortPools(defaultAllPools, cosmWasmPoolConfig.TransmuterCodeIDs, totalTVL, map[uint64]struct{}{
		allPool.BalancerPoolID: {},
	}, logger)

	sortedPoolIDs := getPoolIDs(sortedPools)

	s.Require().Equal(expectedSortedPoolIDs, sortedPoolIDs)
}

// getTakerFeeMapForAllPoolTokenPairs returns a map of all pool token pairs to their taker fees.
func (s *RouterTestSuite) getTakerFeeMapForAllPoolTokenPairs(pools []sqsdomain.PoolI) sqsdomain.TakerFeeMap {
	pairs := make(sqsdomain.TakerFeeMap, 0)

	for _, pool := range pools {
		poolDenoms := pool.GetPoolDenoms()

		for i := 0; i < len(poolDenoms); i++ {
			for j := i + 1; j < len(poolDenoms); j++ {

				hasTakerFee := pairs.Has(poolDenoms[i], poolDenoms[j])
				if hasTakerFee {
					continue
				}

				takerFee, err := s.App.PoolManagerKeeper.GetTradingPairTakerFee(s.Ctx, poolDenoms[i], poolDenoms[j])
				s.Require().NoError(err)

				pairs.SetTakerFee(poolDenoms[i], poolDenoms[j], takerFee)
			}
		}
	}

	return pairs
}

func WithRoutePools(r route.RouteImpl, pools []domain.RoutablePool) route.RouteImpl {
	return routertesting.WithRoutePools(r, pools)
}

func WithCandidateRoutePools(r sqsdomain.CandidateRoute, pools []sqsdomain.CandidatePool) sqsdomain.CandidateRoute {
	return routertesting.WithCandidateRoutePools(r, pools)
}

// getPoolIDs returns the pool IDs of the given pools
func getPoolIDs(pools []sqsdomain.PoolI) []uint64 {
	sortedPoolIDs := make([]uint64, len(pools))
	for i, pool := range pools {
		sortedPoolIDs[i] = pool.GetId()
	}
	return sortedPoolIDs
}
