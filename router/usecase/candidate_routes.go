package usecase

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"go.uber.org/zap"
)

// candidatePoolWrapper is an intermediary internal data
// structure for constructing all candidate routes related data.
// It contains pool denoms for validation after the initial route selection.
type candidatePoolWrapper struct {
	sqsdomain.CandidatePool
	PoolDenoms []string
	Idx        int
}

// GetCandidateRoutes returns candidate routes from tokenInDenom to tokenOutDenom using BFS.
// TODO: Build a better algorithm for finding routes.
// Right now we iterate over the etnire sorted list to try building routes. But most of the work is wasted
// in every iteration, as we have to think about pools that won't relate to the needed asset.
// instead we should have in router:
// * sortedPoolsByDenom map[string][]sqsdomain.PoolI. Where the return value is all pools that contain the denom, sorted.
//   - Right now we have linear time iteration per route rather than N^2 by making every route get created in sorted order.
//   - We can do similar here by actually making the value of the hashmap be a []struct{global sort index, sqsdomain pool}
func GetCandidateRoutes(pools []sqsdomain.PoolI, tokenIn sdk.Coin, tokenOutDenom string, maxRoutes, maxPoolsPerRoute int, logger log.Logger) (sqsdomain.CandidateRoutes, error) {
	routes := make([][]candidatePoolWrapper, 0, maxRoutes)
	// Preallocate third to avoid dynamic reallocations.
	visited := make([]bool, len(pools))

	// Preallocate third of the pools to avoid dynamic reallocations.
	queue := make([][]candidatePoolWrapper, 0, len(pools)/3)
	queue = append(queue, make([]candidatePoolWrapper, 0, maxPoolsPerRoute))

	for len(queue) > 0 && len(routes) < maxRoutes {
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

		for i := 0; i < len(pools) && len(routes) < maxRoutes; i++ {
			// Unsafe cast for performance reasons.
			// nolint: forcetypeassert
			pool := (pools[i]).(*sqsdomain.PoolWrapper)
			poolID := pool.ChainModel.GetId()

			if visited[i] {
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
				isAlloyed := pool.SQSModel.CosmWasmPoolModel != nil && pool.SQSModel.CosmWasmPoolModel.IsAlloyTransmuter()

				if currentTokenInAmount.LT(tokenIn.Amount) && !isAlloyed {
					visited[i] = true
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

				if lastPoolID == uint64(0) || lastPoolID != currentPoolID {
					newPath := make([]candidatePoolWrapper, len(currentRoute), len(currentRoute)+1)

					copy(newPath, currentRoute)

					newPath = append(newPath, candidatePoolWrapper{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            poolID,
							TokenOutDenom: denom,
						},
						PoolDenoms: poolDenoms,
						Idx:        i,
					})

					if len(newPath) <= maxPoolsPerRoute {
						if hasTokenOut {
							routes = append(routes, newPath)
							break
						} else {
							queue = append(queue, newPath)
						}
					}
				}
			}
		}

		for _, pool := range currentRoute {
			visited[pool.Idx] = true
		}
	}

	return validateAndFilterRoutes(routes, tokenIn.Denom, logger)
}

// GetCandidateRoutesNew new algorithm for demo purposes.
// Note: implementation is for demo purposes and is to be further optimized.
// TODO: spec, unit tests via https://linear.app/osmosis/issue/DATA-250/[candidaterouteopt]-reimplement-and-test-getcandidateroute-algorithm
func GetCandidateRoutesNew(poolsByDenom map[string][]sqsdomain.PoolI, tokenIn sdk.Coin, tokenOutDenom string, maxRoutes, maxPoolsPerRoute int, minPoolLiquidityCap uint64, logger log.Logger) (sqsdomain.CandidateRoutes, error) {
	routes := make([][]candidatePoolWrapper, 0, maxRoutes)

	// Preallocate constant visited map size to avoid reallocations.
	// TODO: choose the best size for the visited map.
	visited := make(map[uint64]struct{}, 100)
	// visited := make([]bool, len(pools))

	// Preallocate constant queue size to avoid dynamic reallocations.
	// TODO: choose the best size for the queue.
	queue := make([][]candidatePoolWrapper, 0, 100)
	queue = append(queue, make([]candidatePoolWrapper, 0, maxPoolsPerRoute))

	for len(queue) > 0 && len(routes) < maxRoutes {
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

		rankedPools, ok := poolsByDenom[currenTokenInDenom]
		if !ok {
			return sqsdomain.CandidateRoutes{}, fmt.Errorf("no pools found for denom %s", currenTokenInDenom)
		}

		for i := 0; i < len(rankedPools) && len(routes) < maxRoutes; i++ {
			// Unsafe cast for performance reasons.
			// nolint: forcetypeassert
			pool := (rankedPools[i]).(*sqsdomain.PoolWrapper)
			poolID := pool.ChainModel.GetId()

			if _, ok := visited[poolID]; ok {
				continue
			}

			if pool.GetLiquidityCap().Uint64() < minPoolLiquidityCap {
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
				isAlloyed := pool.SQSModel.CosmWasmPoolModel != nil && pool.SQSModel.CosmWasmPoolModel.IsAlloyTransmuter()

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

				_, ok := poolsByDenom[denom]
				if !ok {
					logger.Debug("no pools found for denom in candidate route search", zap.String("denom", denom))
					continue
				}

				if lastPoolID == uint64(0) || lastPoolID != currentPoolID {
					newPath := make([]candidatePoolWrapper, len(currentRoute), len(currentRoute)+1)

					copy(newPath, currentRoute)

					newPath = append(newPath, candidatePoolWrapper{
						CandidatePool: sqsdomain.CandidatePool{
							ID:            poolID,
							TokenOutDenom: denom,
						},
						PoolDenoms: poolDenoms,
						Idx:        i,
					})

					if len(newPath) <= maxPoolsPerRoute {
						if hasTokenOut {
							routes = append(routes, newPath)
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

	return validateAndFilterRoutes(routes, tokenIn.Denom, logger)
}

// Pool represents a pool in the decentralized exchange.
type Pool struct {
	ID       int
	TokenIn  string
	TokenOut string
}
