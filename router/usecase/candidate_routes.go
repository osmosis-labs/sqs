package usecase

import (
	"github.com/osmosis-labs/sqs/sqsdomain"
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
func (r Router) GetCandidateRoutes(tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, error) {
	routes := make([][]candidatePoolWrapper, 0, r.config.MaxRoutes)
	// Preallocate third to avoid dynamic reallocations.
	visited := make([]bool, len(r.sortedPools))

	// Preallocate tenth of the pools to avoid dynamic reallocations.
	queue := make([][]candidatePoolWrapper, 0, len(r.sortedPools)/10)
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

			// Unsafe cast for performance reasons.
			pool := (r.sortedPools[i]).(*sqsdomain.PoolWrapper)
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
						Idx:        i,
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
			visited[pool.Idx] = true
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
