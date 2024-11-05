package mvc

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// CandidateRouteSearchDataUpdateListener is the interface for the candidate route search data holder.
type CandidateRouteSearchDataHolder interface {
	// SetCandidateRouteSearchData sets the candidate route search data on the holder
	SetCandidateRouteSearchData(candidateRouteSearchData map[string]domain.CandidateRouteDenomData)

	// GetCandidateRouteSearchData gets the candidate route search data from the holder
	GetCandidateRouteSearchData() map[string]domain.CandidateRouteDenomData

	// GetDenomData returns the ranked candidate route search pool data for a given denom.
	// Returns an empty struct if the denom is not found.
	// Returns error if retrieved pools are not of type sqsdomain.PoolI.
	GetDenomData(denom string) (domain.CandidateRouteDenomData, error)
}

// RouterRepository represents the contract for a repository handling tokens information
type RouterRepository interface {
	CandidateRouteSearchDataHolder

	// GetTakerFee returns the taker fee for a given pair of denominations
	// Sorting is no longer performed before looking up as bi-directional taker fees are stored.
	// Returns true if the taker fee for a given denomimnation is found. False otherwise.
	GetTakerFee(denom0, denom1 string) (osmomath.Dec, bool)
	// GetAllTakerFees returns all taker fees
	GetAllTakerFees() sqsdomain.TakerFeeMap
	// SetTakerFee sets the taker fee for a given pair of denominations
	// Sorting is no longer performed before storing as bi-directional taker fee is supported.
	SetTakerFee(denom0, denom1 string, takerFee osmomath.Dec)
	// SetTakerFees sets taker fees on router repository
	SetTakerFees(takerFees sqsdomain.TakerFeeMap)

	GetBaseFee() domain.BaseFee
}

// SimpleRouterUsecase represent the simple router's usecases
// if getting a simple quote and a pool spot price.
type SimpleRouterUsecase interface {
	// GetSimpleQuote returns a simple quote for the given tokenIn and tokenOutDenom.
	// No split routes or caching is used.
	GetSimpleQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error)

	// GetPoolSpotPrice returns the spot price of a pool.
	GetPoolSpotPrice(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error)
}

// RouterUsecase represent the router's usecases
type RouterUsecase interface {
	SimpleRouterUsecase

	// GetOptimalQuote returns the optimal quote for the given tokenIn and tokenOutDenom.
	GetOptimalQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error)

	// GetOptimalQuoteInGivenOut returns the optimal quote for the given token swap method exact amount out.
	GetOptimalQuoteInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, opts ...domain.RouterOption) (domain.Quote, error)

	// GetCustomDirectQuote returns the custom direct quote for the given tokenIn, tokenOutDenom and poolID.
	// It does not search for the route. It directly computes the quote for the given poolID.
	// This allows to bypass a min liquidity requirement in the router when attempting to swap over a specific pool.
	GetCustomDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error)
	// GetCustomDirectQuoteMultiPool calculates direct custom quote for given tokenIn and tokenOutDenom over given poolID route.
	// Underlying implementation uses GetCustomDirectQuote.
	GetCustomDirectQuoteMultiPool(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error)
	// GetCustomDirectQuoteMultiPool calculates direct custom quote for given tokenOut and tokenInDenom over given poolID route.
	// Underlying implementation uses GetCustomDirectQuote.
	GetCustomDirectQuoteMultiPoolInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error)
	// GetCandidateRoutes returns the candidate routes for the given tokenIn and tokenOutDenom.
	GetCandidateRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sqsdomain.CandidateRoutes, error)

	GetBaseFee() domain.BaseFee

	// GetTakerFee returns the taker fee for all token pairs in a pool.
	GetTakerFee(poolID uint64) ([]sqsdomain.TakerFeeForPair, error)
	// SetTakerFees sets the taker fees for all token pairs in all pools.
	SetTakerFees(takerFees sqsdomain.TakerFeeMap)
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

	// GetMinPoolLiquidityCapFilter returns the min pool liquidity capitalization filter for the given tokenIn and tokenOutDenom.
	// It is used to filter out pools with liquidity less than the output of this function.
	// Returns error if one of the denom metadata is not found.
	// Returns error if the filter is not found for the given denoms.
	GetMinPoolLiquidityCapFilter(tokenInDenom, tokenOutDenom string) (uint64, error)

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
