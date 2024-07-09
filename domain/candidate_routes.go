package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

// CandidateRouteSearchOptions represents the options for finding candidate routes.
type CandidateRouteSearchOptions struct {
	// MaxRoutes is the maximum number of routes to find.
	MaxRoutes           int
	// MaxPoolsPerRoute is the maximum number of pools to consider for each route.
	MaxPoolsPerRoute    int
	// MinPoolLiquidityCap is the minimum liquidity cap for a pool to be considered.
	MinPoolLiquidityCap uint64
}

// CandidateRouteSearcher is the interface for finding candidate routes.
type CandidateRouteSearcher interface {
	// FindCandidateRoutes finds candidate routes for a given tokenIn and tokenOutDenom
	// using the given options.
	// Returns the candidate routes and an error if any.
	FindCandidateRoutes(tokenIn sdk.Coin, tokenOutDenom string, options CandidateRouteSearchOptions) (sqsdomain.CandidateRoutes, error)
}
