package mvc

import (
	"context"

	"github.com/osmosis-labs/sqsdomain"

	"github.com/osmosis-labs/sqs/router/usecase/route"
)

// PoolsUsecase represent the pool's usecases
type PoolsUsecase interface {
	GetAllPools(ctx context.Context) ([]sqsdomain.PoolI, error)

	// GetRoutesFromCandidates converts candidate routes to routes intrusmented with all the data necessary for estimating
	// a swap. This data entails the pool data, the taker fee.
	GetRoutesFromCandidates(ctx context.Context, candidateRoutes route.CandidateRoutes, takerFeeMap sqsdomain.TakerFeeMap, tokenInDenom, tokenOutDenom string) ([]route.RouteImpl, error)

	GetTickModelMap(ctx context.Context, poolIDs []uint64) (map[uint64]sqsdomain.TickModel, error)
	// GetPool returns the pool with the given ID.
	GetPool(ctx context.Context, poolID uint64) (sqsdomain.PoolI, error)
}
