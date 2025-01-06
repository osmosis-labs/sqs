package usecase

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

// candidatePoolWrapper is an intermediary internal data
// structure for constructing all candidate routes related data.
// It contains pool denoms for validation after the initial route selection.
type candidatePoolWrapper struct {
	ingesttypes.CandidatePool
	PoolDenoms []string
}

type candidateRouteWrapper struct {
	Pools                     []candidatePoolWrapper
	IsCanonicalOrderboolRoute bool
}

type candidateRouteFinder struct {
	candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder
	logger                   log.Logger
}

var _ domain.CandidateRouteSearcher = candidateRouteFinder{}

func NewCandidateRouteFinder(candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder, logger log.Logger) candidateRouteFinder {
	return candidateRouteFinder{
		candidateRouteDataHolder: candidateRouteDataHolder,
		logger:                   logger,
	}
}

// FindCandidateRoutes implements domain.CandidateRouteFinder.
func (c candidateRouteFinder) FindCandidateRoutes(tokenIn sdk.Coin, tokenOutDenom string, options domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	routes := make([]candidateRouteWrapper, 0, options.MaxRoutes)

	// Preallocate constant visited map size to avoid reallocations.
	// TODO: choose the best size for the visited map.
	visited := make(map[uint64]struct{}, 100)
	// visited := make([]bool, len(pools))

	// Preallocate constant queue size to avoid dynamic reallocations.
	// TODO: choose the best size for the queue.
	queue := make([][]candidatePoolWrapper, 0, 100)
	queue = append(queue, make([]candidatePoolWrapper, 0, options.MaxPoolsPerRoute))

	denomData, err := c.candidateRouteDataHolder.GetDenomData(tokenIn.Denom)
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	if len(denomData.CanonicalOrderbooks) > 0 {
		canonicalOrderbook, ok := denomData.CanonicalOrderbooks[tokenOutDenom]
		if ok {
			shouldSkipCanonicalOrderbook := false
			// Filter the canonical orderbook pool using the pool filters.
			for _, filter := range options.PoolFiltersAnyOf {
				// nolint: forcetypeassert
				canonicalOrderbookPoolWrapper := (canonicalOrderbook).(*ingesttypes.PoolWrapper)
				if filter(canonicalOrderbookPoolWrapper) {
					shouldSkipCanonicalOrderbook = true
					break
				}
			}

			if !shouldSkipCanonicalOrderbook {
				// Add the canonical orderbook as a route.
				routes = append(routes, candidateRouteWrapper{
					IsCanonicalOrderboolRoute: true,
					Pools: []candidatePoolWrapper{
						{
							CandidatePool: ingesttypes.CandidatePool{
								ID:            canonicalOrderbook.GetId(),
								TokenOutDenom: tokenOutDenom,
							},
							PoolDenoms: canonicalOrderbook.GetSQSPoolModel().PoolDenoms,
						},
					},
				})
			}

			visited[canonicalOrderbook.GetId()] = struct{}{}
		}
	}

	for len(queue) > 0 && len(routes) < options.MaxRoutes {
		currentRoute := queue[0]
		queue[0] = nil // Clear the slice to avoid holding onto references
		queue = queue[1:]

		lastPoolID := uint64(0)
		currenTokenInDenom := tokenIn.Denom
		if len(currentRoute) > 0 {
			lastPool := currentRoute[len(currentRoute)-1]
			lastPoolID = lastPool.ID
			currenTokenInDenom = lastPool.TokenOutDenom
		}

		denomData, err := c.candidateRouteDataHolder.GetDenomData(currenTokenInDenom)
		if err != nil {
			return ingesttypes.CandidateRoutes{}, err
		}

		rankedPools := denomData.SortedPools

		if len(rankedPools) == 0 {
			c.logger.Debug("no pools found for denom in candidate route search", zap.String("denom", currenTokenInDenom))
		}

		for i := 0; i < len(rankedPools) && len(routes) < options.MaxRoutes; i++ {
			// Unsafe cast for performance reasons.
			// nolint: forcetypeassert
			pool := (rankedPools[i]).(*ingesttypes.PoolWrapper)
			poolID := pool.ChainModel.GetId()

			if _, ok := visited[poolID]; ok {
				continue
			}

			// If the option is configured to skip a given pool
			// We mark it as visited and continue.
			if options.ShouldSkipPool(pool) {
				visited[poolID] = struct{}{}
				continue
			}

			if pool.GetLiquidityCap().Uint64() < options.MinPoolLiquidityCap {
				visited[poolID] = struct{}{}
				// Skip pools that have less liquidity than the minimum required.
				continue
			}

			poolDenoms := pool.SQSModel.PoolDenoms
			hasTokenIn := false
			hasTokenOut := false
			shouldSkipPool := false
			for _, denom := range poolDenoms {
				if denom == currenTokenInDenom {
					hasTokenIn = true
				}
				if denom == tokenOutDenom {
					hasTokenOut = true
				}

				// Avoid going through pools that has the initial token in denom twice.
				if len(currentRoute) > 0 && denom == tokenIn.Denom {
					shouldSkipPool = true
					break
				}
			}

			if shouldSkipPool {
				continue
			}

			if !hasTokenIn {
				continue
			}

			// Microptimization for the first pool in the route.
			if len(currentRoute) == 0 {
				currentTokenInAmount := pool.SQSModel.Balances.AmountOf(currenTokenInDenom)

				// HACK: alloyed LP share is not contained in balances.
				// TODO: remove the hack and ingest the LP share balance on the Osmosis side.
				// https://linear.app/osmosis/issue/DATA-236/bug-alloyed-lp-share-is-not-present-in-balances
				cosmwasmModel := pool.SQSModel.CosmWasmPoolModel
				isAlloyed := cosmwasmModel != nil && cosmwasmModel.IsAlloyTransmuter()

				if currentTokenInAmount.LT(tokenIn.Amount) && !isAlloyed {
					visited[poolID] = struct{}{}
					// Not enough tokenIn to swap.
					continue
				}
			}

			currentPoolID := poolID
			for _, denom := range poolDenoms {
				if denom == currenTokenInDenom {
					continue
				}
				if hasTokenOut && denom != tokenOutDenom {
					continue
				}

				denomData, err := c.candidateRouteDataHolder.GetDenomData(currenTokenInDenom)
				if err != nil {
					return ingesttypes.CandidateRoutes{}, err
				}

				rankedPools := denomData.SortedPools
				if len(rankedPools) == 0 {
					c.logger.Debug("no pools found for denom in candidate route search", zap.String("denom", denom))
					continue
				}

				if lastPoolID == uint64(0) || lastPoolID != currentPoolID {
					newPath := make([]candidatePoolWrapper, len(currentRoute), len(currentRoute)+1)

					copy(newPath, currentRoute)

					newPath = append(newPath, candidatePoolWrapper{
						CandidatePool: ingesttypes.CandidatePool{
							ID:            poolID,
							TokenOutDenom: denom,
						},
						PoolDenoms: poolDenoms,
					})

					if len(newPath) <= options.MaxPoolsPerRoute {
						if hasTokenOut {
							routes = append(routes, candidateRouteWrapper{
								Pools:                     newPath,
								IsCanonicalOrderboolRoute: false,
							})
							break
						} else {
							queue = append(queue, newPath)
						}
					}
				}
			}
		}

		for _, pool := range currentRoute {
			visited[pool.ID] = struct{}{}
		}
	}

	return validateAndFilterRoutes(routes, tokenIn.Denom, c.logger)
}

// Pool represents a pool in the decentralized exchange.
type Pool struct {
	ID       int
	TokenIn  string
	TokenOut string
}
