package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
)

// CandidateRoutePoolFiltrerCb defines a candidate route pool filter
// that takes in a pool and returns true if the pool should be skipped.
type CandidateRoutePoolFiltrerCb func(*ingesttypes.PoolWrapper) bool

// CandidateRouteSearchOptions represents the options for finding candidate routes.
type CandidateRouteSearchOptions struct {
	// MaxRoutes is the maximum number of routes to find.
	MaxRoutes int
	// MaxPoolsPerRoute is the maximum number of pools to consider for each route.
	MaxPoolsPerRoute int
	// MinPoolLiquidityCap is the minimum liquidity cap for a pool to be considered.
	MinPoolLiquidityCap uint64
	// DisableCache specifies if route cache should be disbled.
	// If true, the candidate route cache is neither read nor written to.
	DisableCache bool

	// PoolFiltersAnyOf are the callbacks that take in a pool, returning
	// true if the candidate route algorithm should ignore a pool matching a certain condition.
	// If at least one of the callbacks in-slice returns true, the ShouldSkipPool function will
	// also return true.
	PoolFiltersAnyOf []CandidateRoutePoolFiltrerCb
}

// ShouldSkipPool returns true if the candidate route algorithm should skip
// a given pool by matching at least one of the pool filters
func (c CandidateRouteSearchOptions) ShouldSkipPool(pool *ingesttypes.PoolWrapper) bool {
	for _, filter := range c.PoolFiltersAnyOf {
		if filter(pool) {
			return true
		}
	}
	return false
}

// CandidateRoutePoolIDFilterOptionCb encapsulates the pool IDs that should be skipped by the candidate route
// algorithm, exposing an API to determine whether the given pool mathes any of the pool IDs that
// should be skipped.
type CandidateRoutePoolIDFilterOptionCb struct {
	PoolIDsToSkip map[uint64]struct{}
}

// ShouldSkipPool returns true of the given pool has ID that is present in c.PoolIDsToSkip
func (c CandidateRoutePoolIDFilterOptionCb) ShouldSkipPool(pool *ingesttypes.PoolWrapper) bool {
	poolID := pool.GetId()
	_, ok := c.PoolIDsToSkip[poolID]
	return ok
}

var (
	// ShouldSkipOrderbookPool skips orderbook pools
	// by returning true if pool.SQSModel.CosmWasmPoolModel is not nil
	// and pool.SQSModel.CosmWasmPoolModel.IsOrderbook() returns true.
	ShouldSkipOrderbookPool CandidateRoutePoolFiltrerCb = func(pool *ingesttypes.PoolWrapper) bool {
		cosmWasmPoolModel := pool.SQSModel.CosmWasmPoolModel
		return cosmWasmPoolModel != nil && cosmWasmPoolModel.IsOrderbook()
	}
)

// CandidateRouteSearcher is the interface for finding candidate routes.
type CandidateRouteSearcher interface {
	// FindCandidateRoutes finds candidate routes for a given tokenIn and tokenOutDenom
	// using the given options.
	// Returns the candidate routes and an error if any.
	FindCandidateRoutes(tokenIn sdk.Coin, tokenOutDenom string, options CandidateRouteSearchOptions) (ingesttypes.CandidateRoutes, error)
}

// CandidateRouteDenomData represents the data for a candidate route for a given denom.
type CandidateRouteDenomData struct {
	// SortedPools is the sorted list of pools for the denom.
	SortedPools []ingesttypes.PoolI
	// CanonicalOrderbooks is the map of canonical orderbooks keyed by the pair token.
	// For example if this is candidate route denom data for OSMO and there is a canonical orderbook with ID 23
	// for ATOM/OSMO, we would have an entry from ATOM to 23 in this map.
	CanonicalOrderbooks map[string]ingesttypes.PoolI
}
