package usecase

import (
	poolmanagertypes "github.com/osmosis-labs/osmosis/v24/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

// candidatePoolWrapper is an intermediary internal data
// structure for constructing all candidate routes related data.
// It contains pool denoms for validation after the initial route selection.
// Additionally, it contains the pool type for contracting eventually constructing
// a unque list of concentrated pools for knowing which pools require
// a tick model.
type candidatePoolWrapper struct {
	sqsdomain.CandidatePool
	PoolDenoms []string
	PoolType   poolmanagertypes.PoolType
}

// GetCandidateRoutes returns candidate routes from tokenInDenom to tokenOutDenom using BFS.
// TODO: Build a better algorithm for finding routes.
// Right now we iterate over the etnire sorted list to try building routes. But most of the work is wasted
// in every iteration, as we have to think about pools that won't relate to the needed asset.
// instead we should have in router:
// * sortedPoolsByDenom map[string][]sqsdomain.PoolI. Where the return value is all pools that contain the denom, sorted.
//   - Right now we have linear time iteration per route rather than N^2 by making every route get created in sorted order.
//   - We can do similar here by actually making the value of the hashmap be a []struct{global sort index, sqsdomain pool}
func (r Router) GetCandidateRoutes(tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, error) {
	var routes [][]candidatePoolWrapper
	var visited = make(map[uint64]bool)

	queue := make([][]candidatePoolWrapper, 0)
	queue = append(queue, []candidatePoolWrapper{})

	for len(queue) > 0 && len(routes) < r.config.MaxRoutes {
		currentRoute := queue[0]
		queue = queue[1:]

		lastPoolID := uint64(0)
		currenTokenInDenom := tokenInDenom
		if len(currentRoute) > 0 {
			lastPool := currentRoute[len(currentRoute)-1]
			lastPoolID = lastPool.ID
			currenTokenInDenom = lastPool.TokenOutDenom
		}

		for i := 0; i < len(r.sortedPools) && len(routes) < r.config.MaxRoutes; i++ {
			pool := r.sortedPools[i]
			poolID := pool.GetId()

			if visited[poolID] {
				continue
			}

			poolDenoms := pool.GetPoolDenoms()
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
				if len(currentRoute) > 0 && denom == tokenInDenom {
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
						PoolType:   pool.GetType(),
					})

					if len(newPath) <= r.config.MaxPoolsPerRoute {
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
			visited[pool.ID] = true
		}
	}

	return r.validateAndFilterRoutes(routes, tokenInDenom)
}

// Pool represents a pool in the decentralized exchange.
type Pool struct {
	ID       int
	TokenIn  string
	TokenOut string
}
