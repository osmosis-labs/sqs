package usecase

import (
	"context"
	"sync"

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

// candidateRouteNode is a queued path in the candidate route search: the path's last pool and the
// index of the node holding the path without it, so that queued paths share their prefixes instead
// of each holding a copy. A parent of -1 means the path has only this pool.
type candidateRouteNode struct {
	pool   candidatePoolWrapper
	parent int
	length int
}

// candidateRoutePath returns the pools of the path ending at node index last, followed by next.
// last may be -1 for the empty path.
func candidateRoutePath(nodes []candidateRouteNode, last int, next candidatePoolWrapper) []candidatePoolWrapper {
	length := 1
	if last >= 0 {
		length += nodes[last].length
	}

	path := make([]candidatePoolWrapper, length)
	path[length-1] = next
	for i, j := last, length-2; i >= 0; i, j = nodes[i].parent, j-1 {
		path[j] = nodes[i].pool
	}

	return path
}

// visitedPoolsSize bounds the pool IDs the candidate route search can mark as visited.
const visitedPoolsSize = 100_000

// visitedPools reuses the visited slices of finished searches, which are cleared before reuse.
var visitedPools = sync.Pool{
	New: func() any {
		visited := make([]bool, visitedPoolsSize)
		return &visited
	},
}

type candidateRouteFinder struct {
	candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder
	// excludedPools are skipped by every search regardless of the per-call options,
	// so that neither routing nor pricing can produce a route through them.
	excludedPools domain.CandidateRoutePoolIDFilterOptionCb
	logger        log.Logger
}

var _ domain.CandidateRouteSearcher = candidateRouteFinder{}

func NewCandidateRouteFinder(candidateRouteDataHolder mvc.CandidateRouteSearchDataHolder, excludedPoolIDs []uint64, logger log.Logger) candidateRouteFinder {
	poolIDsToSkip := make(map[uint64]struct{}, len(excludedPoolIDs))
	for _, poolID := range excludedPoolIDs {
		poolIDsToSkip[poolID] = struct{}{}
	}

	return candidateRouteFinder{
		candidateRouteDataHolder: candidateRouteDataHolder,
		excludedPools:            domain.CandidateRoutePoolIDFilterOptionCb{PoolIDsToSkip: poolIDsToSkip},
		logger:                   logger,
	}
}

// FindCandidateRoutesOutGivenIn implements domain.CandidateRouteSearcher.
func (c candidateRouteFinder) FindCandidateRoutesOutGivenIn(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, options domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	routes := make([]candidateRouteWrapper, 0, options.MaxRoutes)

	visitedPtr := visitedPools.Get().(*[]bool)
	visited := *visitedPtr
	defer func() {
		clear(visited)
		visitedPools.Put(visitedPtr)
	}()

	// The queue holds indexes into nodes, starting with the empty path (-1).
	// TODO: choose the best initial size for the queue.
	nodes := make([]candidateRouteNode, 0, 100)
	queue := make([]int, 0, 100)
	queue = append(queue, -1)

	denomData, err := c.candidateRouteDataHolder.GetDenomData(tokenIn.Denom)
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	if len(denomData.CanonicalOrderbooks) > 0 {
		canonicalOrderbook, ok := denomData.CanonicalOrderbooks[tokenOutDenom]
		if ok {
			shouldSkipCanonicalOrderbook := c.excludedPools.ShouldSkipPool(canonicalOrderbook)
			// Filter the canonical orderbook pool using the pool filters.
			for _, filter := range options.PoolFiltersAnyOf {
				if filter(canonicalOrderbook) {
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
							ID:            canonicalOrderbook.ID,
							TokenInDenom:  tokenIn.Denom,
							TokenOutDenom: tokenOutDenom,
							PoolDenoms:    canonicalOrderbook.PoolDenoms,
						},
					},
				})
			}

			visited[canonicalOrderbook.ID] = true
		}
	}

	for head := 0; head < len(queue) && len(routes) < options.MaxRoutes; head++ {
		currentNode := queue[head]

		currentRouteLen := 0
		lastPoolID := uint64(0)
		currenTokenInDenom := tokenIn.Denom
		if currentNode >= 0 {
			lastPool := nodes[currentNode].pool
			currentRouteLen = nodes[currentNode].length
			lastPoolID = lastPool.ID
			currenTokenInDenom = lastPool.TokenOutDenom
		}

		denomData, err := c.candidateRouteDataHolder.GetDenomData(currenTokenInDenom)
		if err != nil {
			return ingesttypes.CandidateRoutes{}, err
		}

		if len(denomData.SortedPools) == 0 {
			c.logger.Debug("no pools found for denom in candidate route search", zap.String("denom", currenTokenInDenom))
		}

		for i := 0; i < len(denomData.SortedPools) && len(routes) < options.MaxRoutes; i++ {
			if ok := visited[denomData.SortedPools[i].ID]; ok {
				continue
			}

			pool := (denomData.SortedPools[i])

			// If the pool is excluded by config or the option is configured to skip it,
			// we mark it as visited and continue.
			if c.excludedPools.ShouldSkipPool(pool) || options.ShouldSkipPool(pool) {
				visited[pool.ID] = true
				continue
			}

			// Skip pools with insufficient liquidity, except for transmuter pools
			// Transmuter pools should always be considered regardless of liquidity cap
			// because they provide no-slippage swaps and we want them prioritized
			if pool.GetPoolLiquidityCap() < options.MinPoolLiquidityCap && !pool.IsAlloyTransmuter {
				visited[pool.ID] = true
				// Skip pools that have less liquidity than the minimum required.
				continue
			}

			poolDenoms := pool.PoolDenoms
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
				if currentRouteLen > 0 && denom == tokenIn.Denom {
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
			if currentRouteLen == 0 {
				currentTokenInAmount := pool.Balances.AmountOf(currenTokenInDenom)

				// HACK: alloyed LP share is not contained in balances.
				// TODO: remove the hack and ingest the LP share balance on the Osmosis side.
				// https://linear.app/osmosis/issue/DATA-236/bug-alloyed-lp-share-is-not-present-in-balances
				if currentTokenInAmount.LT(tokenIn.Amount) && !pool.IsAlloyTransmuter {
					visited[pool.ID] = true
					// Not enough tokenIn to swap.
					continue
				}
			}

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

				if len(denomData.SortedPools) == 0 {
					c.logger.Debug("no pools found for denom in candidate route search", zap.String("denom", denom))
					continue
				}

				if lastPoolID == uint64(0) || lastPoolID != pool.ID {
					nextPool := candidatePoolWrapper{
						ID:            pool.ID,
						TokenInDenom:  currenTokenInDenom,
						TokenOutDenom: denom,
						PoolDenoms:    poolDenoms,
					}

					if currentRouteLen+1 <= options.MaxPoolsPerRoute {
						if hasTokenOut {
							routes = append(routes, candidateRouteWrapper{
								Pools:                     candidateRoutePath(nodes, currentNode, nextPool),
								IsCanonicalOrderboolRoute: false,
							})
							break
						} else {
							nodes = append(nodes, candidateRouteNode{pool: nextPool, parent: currentNode, length: currentRouteLen + 1})
							queue = append(queue, len(nodes)-1)
						}
					}
				}
			}
		}

		for i := currentNode; i >= 0; i = nodes[i].parent {
			visited[nodes[i].pool.ID] = true
		}
	}

	return validateAndFilterRoutesOutGivenIn(routes, tokenIn.Denom, c.logger)
}

// FindCandidateRoutesInGivenOut implements domain.CandidateRouteSearcher.
func (c candidateRouteFinder) FindCandidateRoutesInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, options domain.CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error) {
	// Fetching the candidate routes as for the exact amount of token in swap method
	// That will be the same as the exact amount out swap method with inverted token denominations
	routes, err := c.FindCandidateRoutesOutGivenIn(ctx, tokenOut, tokenInDenom, options)
	if err != nil {
		return ingesttypes.CandidateRoutes{}, err
	}

	// Inverting token denominations for each route to match exact amount out swap method
	for i, v := range routes.Routes {
		for j := range v.Pools {
			routes.Routes[i].Pools[j].TokenInDenom, routes.Routes[i].Pools[j].TokenOutDenom = routes.Routes[i].Pools[j].TokenOutDenom, routes.Routes[i].Pools[j].TokenInDenom
		}
	}

	return routes, nil
}
