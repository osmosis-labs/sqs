package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase"
	routerusecase "github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/route"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type RouterTestSuite struct {
	routertesting.RouterTestHelper
}

func TestRouterTestSuite(t *testing.T) {
	suite.Run(t, new(RouterTestSuite))
}

const dummyTotalValueLockedErrorStr = "total value locked error string"

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
func (s *RouterTestSuite) TestNewRouter() {
	s.Setup()

	// Prepare all supported pools.
	allPool := s.PrepareAllSupportedPoolsCustomProject("sqs", "scripts")

	// Create additional pools for edge cases
	var (
		secondBalancerPoolPoolID = s.PrepareBalancerPool()
		thirdBalancerPoolID      = s.PrepareBalancerPool()

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
		preferredPoolIDs   = []uint64{allPool.BalancerPoolID}
		maxHops            = 3
		maxRoutes          = 5
		maxSplitRoutes     = 5
		maxSplitIterations = 10
		minOsmoLiquidity   = 2
		logger, _          = log.NewLogger(false, "", "")
		defaultAllPools    = []sqsdomain.PoolI{
			&sqsdomain.PoolWrapper{
				ChainModel: balancerPool,
				SQSModel: sqsdomain.SQSPool{
					TotalValueLockedUSDC: osmomath.NewInt(5 * usecase.OsmoPrecisionMultiplier), // 5
					PoolDenoms:           defaultDenoms,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: stableswapPool,
				SQSModel: sqsdomain.SQSPool{
					TotalValueLockedUSDC: osmomath.NewInt(int64(minOsmoLiquidity) - 1), // 1
					PoolDenoms:           defaultDenoms,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: concentratedPool,
				SQSModel: sqsdomain.SQSPool{
					TotalValueLockedUSDC: osmomath.NewInt(4 * usecase.OsmoPrecisionMultiplier), // 4
					PoolDenoms:           defaultDenoms,
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
					TotalValueLockedUSDC: osmomath.NewInt(3 * usecase.OsmoPrecisionMultiplier), // 3
					PoolDenoms:           defaultDenoms,
				},
			},

			// Note that the pools below have higher TVL.
			// However, since they have TVL error flag set, they
			// should be sorted after other pools, unless overriden by preferredPoolIDs.
			&sqsdomain.PoolWrapper{
				ChainModel: secondBalancerPool,
				SQSModel: sqsdomain.SQSPool{
					TotalValueLockedUSDC:  osmomath.NewInt(10 * usecase.OsmoPrecisionMultiplier), // 10
					PoolDenoms:            defaultDenoms,
					TotalValueLockedError: dummyTotalValueLockedErrorStr,
				},
			},
			&sqsdomain.PoolWrapper{
				ChainModel: thirdBalancerPool,
				SQSModel: sqsdomain.SQSPool{
					TotalValueLockedUSDC:  osmomath.NewInt(11 * usecase.OsmoPrecisionMultiplier), // 11
					PoolDenoms:            defaultDenoms,
					TotalValueLockedError: dummyTotalValueLockedErrorStr,
				},
			},
		}

		// Expected
		// First, preferred pool IDs, then sorted by TVL.
		expectedSortedPoolIDs = []uint64{
			// Transmuter pool is first due to no slippage swaps
			allPool.CosmWasmPoolID,

			// Balancer is above concentrated pool due to being preferred
			allPool.BalancerPoolID,

			allPool.ConcentratedPoolID,

			thirdBalancerPoolID, // non-preferred pool ID with TVL error flag set

			secondBalancerPoolPoolID, // preferred pool ID with TVL error flag set

		}
	)

	// System under test
	router := routerusecase.NewRouter(domain.RouterConfig{
		PreferredPoolIDs:   preferredPoolIDs,
		MaxPoolsPerRoute:   maxHops,
		MaxRoutes:          maxRoutes,
		MaxSplitRoutes:     maxSplitRoutes,
		MaxSplitIterations: maxSplitIterations,
		MinOSMOLiquidity:   minOsmoLiquidity,
	}, domain.CosmWasmPoolRouterConfig{
		TransmuterCodeIDs: map[uint64]struct{}{
			1: {},
		},
		GeneralCosmWasmCodeIDs: map[uint64]struct{}{},
		NodeURI:                "",
	}, logger)
	router = routerusecase.WithSortedPools(router, defaultAllPools)

	// Assert
	s.Require().Equal(maxHops, router.GetMaxHops())
	s.Require().Equal(maxRoutes, router.GetMaxRoutes())
	s.Require().Equal(maxSplitIterations, router.GetMaxSplitIterations())
	s.Require().Equal(logger, router.GetLogger())
	s.Require().Equal(expectedSortedPoolIDs, router.GetSortedPoolIDs())
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

func WithRoutePools(r route.RouteImpl, pools []sqsdomain.RoutablePool) route.RouteImpl {
	return routertesting.WithRoutePools(r, pools)
}

func WithCandidateRoutePools(r sqsdomain.CandidateRoute, pools []sqsdomain.CandidatePool) sqsdomain.CandidateRoute {
	return routertesting.WithCandidateRoutePools(r, pools)
}
