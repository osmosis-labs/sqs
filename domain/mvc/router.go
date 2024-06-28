package mvc

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

// CandidateRouteSearchDataUpdateListener is the interface for the candidate route search data holder.
type CandidateRouteSearchDataHolder interface {
	// SetCandidateRouteSearchData sets the candidate route search data on the holder
	SetCandidateRouteSearchData(candidateRouteSearchData map[string][]sqsdomain.PoolI)

	// GetCandidateRouteSearchData gets the candidate route search data from the holder
	GetCandidateRouteSearchData() map[string][]sqsdomain.PoolI
}

// RouterUsecase represent the router's usecases
type RouterUsecase interface {
	CandidateRouteSearchDataHolder

	// GetOptimalQuote returns the optimal quote for the given tokenIn and tokenOutDenom.
	GetOptimalQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error)

	// GetSimpleQuote returns a simple quote for the given tokenIn and tokenOutDenom.
	// No split routes or caching is used.
	GetSimpleQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error)
	// GetCustomDirectQuote returns the custom direct quote for the given tokenIn, tokenOutDenom and poolID.
	// It does not search for the route. It directly computes the quote for the given poolID.
	// This allows to bypass a min liquidity requirement in the router when attempting to swap over a specific pool.
	GetCustomDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error)
	// GetCustomDirectQuoteMultiPool calculates direct custom quote for given tokenIn and tokenOutDenom over given poolID route.
	// Otherwise it implements same rules as GetCustomDirectQuote.
	GetCustomDirectQuoteMultiPool(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error)
	// GetCandidateRoutes returns the candidate routes for the given tokenIn and tokenOutDenom.
	GetCandidateRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sqsdomain.CandidateRoutes, error)
	// GetTakerFee returns the taker fee for all token pairs in a pool.
	GetTakerFee(poolID uint64) ([]sqsdomain.TakerFeeForPair, error)
	// SetTakerFees sets the taker fees for all token pairs in all pools.
	SetTakerFees(takerFees sqsdomain.TakerFeeMap)
	// GetPoolSpotPrice returns the spot price of a pool.
	GetPoolSpotPrice(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error)
	// GetCachedCandidateRoutes returns the candidate routes for the given tokenIn and tokenOutDenom from cache.
	// It does not recompute the routes if they are not present in cache.
	// Since we may cache zero routes, it returns false if the routes are not present in cache. Returns true otherwise.
	// Returns error if cache is disabled.
	GetCachedCandidateRoutes(ctx context.Context, tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, bool, error)
	// StoreRoutes stores all router state in the files locally. Used for debugging.
	StoreRouterStateFiles() error

	GetRouterState() (domain.RouterState, error)

	// GetSortedPools returns the sorted pools based on the router configuration.
	GetSortedPools() []sqsdomain.PoolI

	GetConfig() domain.RouterConfig

	// ConvertMinTokensPoolLiquidityCapToFilter converts the minTokensPoolLiquidityCap to a filter.
	// It is used to filter out pools with liquidity less than the output of this function.
	// We use min(tokenInPoolLiquidityCap, tokenOutPoolLiquidityCap) as a proxy for finding the appropriate
	// filter if configured.
	// If there is no entry in the config that has min tokens capitalization smaller than the given value,
	// the default router min pool liquidity capitalization is returned.
	ConvertMinTokensPoolLiquidityCapToFilter(minTokensPoolLiquidityCap uint64) uint64

	// SetSortedPools stores the pools in the router.
	// CONTRACT: the pools are already sorted according to the desired parameters.
	// See sortPools() function.
	SetSortedPools(pools []sqsdomain.PoolI)
}
