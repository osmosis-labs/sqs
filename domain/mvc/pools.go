package mvc

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/router/usecase/route"
)

// PoolsUsecase represent the pool's usecases
type PoolsUsecase interface {
	PoolHandler

	GetAllPools() ([]sqsdomain.PoolI, error)

	// GetRoutesFromCandidates converts candidate routes to routes intrusmented with all the data necessary for estimating
	// a swap. This data entails the pool data, the taker fee.
	GetRoutesFromCandidates(candidateRoutes sqsdomain.CandidateRoutes, tokenInDenom, tokenOutDenom string) ([]route.RouteImpl, error)

	GetTickModelMap(poolIDs []uint64) (map[uint64]*sqsdomain.TickModel, error)
	// GetPool returns the pool with the given ID.
	GetPool(poolID uint64) (sqsdomain.PoolI, error)
	// GetPoolSpotPrice returns the spot price of the given pool given the taker fee, quote and base assets.
	GetPoolSpotPrice(ctx context.Context, poolID uint64, takerFee osmomath.Dec, quoteAsset, baseAsset string) (osmomath.BigDec, error)

	GetCosmWasmPoolConfig() domain.CosmWasmPoolRouterConfig

	// GetCanonicalOrderbookPool returns the canonical orderbook pool ID for the given base and quote denoms
	// as well as the associated contract ID.
	// Returns error if the pool is not found for the given pair.
	GetCanonicalOrderbookPool(baseDenom, quoteDenom string) (uint64, string, error)

	// GetAllCanonicalOrderbookPoolIDs returns all the canonical orderbook results
	// where each base/quote denom is associated with a default pool ID.
	// Sorts the results by pool ID.
	GetAllCanonicalOrderbookPoolIDs() ([]domain.CanonicalOrderBooksResult, error)

	// IsCanonicalOrderbookPool returns true if the given pool ID is a canonical orderbook pool
	// for some token pair.
	IsCanonicalOrderbookPool(poolID uint64) bool
}

type PoolHandler interface {
	// GetPools returns the pools corresponding to the given options and the total number of pools.
	// If pagination is provided, returns the pools corresponding to the pagination.
	GetPools(opts ...domain.PoolsOption) ([]sqsdomain.PoolI, uint64, error)

	// StorePools stores the given pools in the usecase
	StorePools(pools []sqsdomain.PoolI) error

	// CalcExitCFMMPool estimates the coins returned from redeeming CFMM pool shares given a pool ID and the GAMM shares to convert
	// poolID must be a CFMM pool. Returns error if not.
	CalcExitCFMMPool(poolID uint64, exitingShares osmomath.Int) (sdk.Coins, error)
}

type CandidateRouteSearchPoolHandler interface {
	PoolHandler

	// IsCanonicalOrderbookPool returns true if the given pool ID is a canonical orderbook pool
	// for at least one of the base and quote denoms.
	IsCanonicalOrderbookPool(poolID uint64) bool
}
