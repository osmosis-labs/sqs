package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

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
}

// CandidateRouteSearcher is the interface for finding candidate routes.
type CandidateRouteSearcher interface {
	// FindCandidateRoutes finds candidate routes for a given tokenIn and tokenOutDenom
	// using the given options.
	// Returns the candidate routes and an error if any.
	FindCandidateRoutes(tokenIn sdk.Coin, tokenOutDenom string, options CandidateRouteSearchOptions) (sqsdomain.CandidateRoutes, error)
}

// CandidateRouteDenomData represents the data for a candidate route for a given denom.
type CandidateRouteDenomData struct {
	// SortedPools is the sorted list of pools for the denom.
	SortedPools []sqsdomain.PoolI
	// CanonicalOrderbooks is the map of canonical orderbooks keyed by the pair token.
	// For example if this is candidate route denom data for OSMO and there is a canonical orderbook with ID 23
	// for ATOM/OSMO, we would have an entry from ATOM to 23 in this map.
	CanonicalOrderbooks map[string]sqsdomain.PoolI
}
