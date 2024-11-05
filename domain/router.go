package domain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type RoutableResultPool interface {
	RoutablePool
	GetBalances() sdk.Coins
}

type Route interface {
	// ContainsGeneralizedCosmWasmPool returns true if the route contains a generalized cosmwasm pool.
	// We track whether a route contains a generalized cosmwasm pool
	// so that we can exclude it from split quote logic.
	// The reason for this is that making network requests to chain is expensive.
	// As a result, we want to minimize the number of requests we make.
	ContainsGeneralizedCosmWasmPool() bool

	GetPools() []RoutablePool

	// CalculateTokenOutByTokenIn calculates the token out amount given the token in amount.
	// Returns error if the calculation fails.
	CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error)

	// Returns token out denom of the last pool in the route.
	// If route is empty, returns empty string.
	GetTokenOutDenom() string

	// Returns token in denom of the last pool in the route.
	// If route is empty, returns empty string.
	GetTokenInDenom() string

	// PrepareResultPools strips away unnecessary fields
	// from each pool in the route,
	// leaving only the data needed by client
	// Runs the quote logic one final time to compute the effective spot price.
	// Note that it mutates the route.
	// Computes the spot price of the route.
	// Returns the spot price before swap and effective spot price.
	// The token in is the base token and the token out is the quote token.
	PrepareResultPools(ctx context.Context, tokenIn sdk.Coin, logger log.Logger) ([]RoutablePool, osmomath.Dec, osmomath.Dec, error)

	String() string
}

type SplitRoute interface {
	Route
	GetAmountIn() osmomath.Int
	GetAmountOut() osmomath.Int
}

type Quote interface {
	GetAmountIn() sdk.Coin
	GetAmountOut() osmomath.Int
	GetRoute() []SplitRoute
	GetEffectiveFee() osmomath.Dec
	GetPriceImpact() osmomath.Dec
	GetInBaseOutQuoteSpotPrice() osmomath.Dec

	// PrepareResult mutates the quote to prepare
	// it with the data formatted for output to the client.
	// scalingFactor is the spot price scaling factor according to chain precision.
	// scalingFactor of zero is a valid value. It might occur if we do not have precision information
	// for the tokens. In that case, we invalidate spot price by setting it to zero.
	PrepareResult(ctx context.Context, scalingFactor osmomath.Dec, logger log.Logger) ([]SplitRoute, osmomath.Dec, error)

	// SetQuotePriceInfo sets the quote price info.
	SetQuotePriceInfo(info *TxFeeInfo)

	String() string
}

type DynamicMinLiquidityCapFilterEntry struct {
	MinTokensCap uint64 `mapstructure:"min-tokens-capitalization"`
	FilterValue  uint64 `mapstructure:"filter-value"`
}

// Router-specific configuration
type RouterConfig struct {
	// Pool IDs that are prioritized in the router.
	PreferredPoolIDs []uint64 `mapstructure:"preferred-pool-ids"`

	// Maximum number of pools in one route.
	MaxPoolsPerRoute int `mapstructure:"max-pools-per-route"`

	// Maximum number of routes to search for.
	MaxRoutes int `mapstructure:"max-routes"`

	// Maximum number of routes to split across.
	MaxSplitRoutes int `mapstructure:"max-split-routes"`

	// Minimum liquidity capitalization for a pool to be considered in the router.
	// The denomination assumed is pricing.default-quote-human-denom.
	MinPoolLiquidityCap uint64 `mapstructure:"min-pool-liquidity-cap"`

	// Whether to enable route caching
	RouteCacheEnabled bool `mapstructure:"route-cache-enabled"`

	// The number of milliseconds to cache candidate routes for before expiry.
	CandidateRouteCacheExpirySeconds int `mapstructure:"candidate-route-cache-expiry-seconds"`

	// How long the route is cached for before expiry in seconds.
	RankedRouteCacheExpirySeconds int `mapstructure:"ranked-route-cache-expiry-seconds"`

	// DynamicMinLiquidityCapFiltersAsc is a list of dynamic min liquidity cap filters in descending order.
	DynamicMinLiquidityCapFiltersDesc []DynamicMinLiquidityCapFilterEntry `mapstructure:"dynamic-min-liquidity-cap-filters-desc"`
}

type PoolsConfig struct {
	// Code IDs of Transmuter CosmWasm pools that are supported.
	TransmuterCodeIDs []uint64 `mapstructure:"transmuter-code-ids"`

	// Code IDs of Alloyed Transmuter CosmWasm pools that are supported.
	AlloyedTransmuterCodeIDs []uint64 `mapstructure:"alloyed-transmuter-code-ids"`

	// Code IDs of Orderbook pools that are supported.
	OrderbookCodeIDs []uint64 `mapstructure:"orderbook-code-ids"`

	// Code IDs of generalized CosmWasm pools that are supported.
	// NOTE: that these pools make network requests to chain for quote estimation.
	// As a result, they are excluded from split routes.
	GeneralCosmWasmCodeIDs []uint64 `mapstructure:"general-cosmwasm-code-ids"`
}

const DisableSplitRoutes = 0

type RouterState struct {
	Pools                    []sqsdomain.PoolI
	TakerFees                sqsdomain.TakerFeeMap
	TickMap                  map[uint64]*sqsdomain.TickModel
	AlloyedDataMap           map[uint64]*cosmwasmpool.AlloyTransmuterData
	CandidateRouteSearchData map[string]CandidateRouteDenomData
}

// RouterOptions defines the options for the router
// By default, the router config that is defined on the router usecase is set.
// The caller of GetQuote(...) may overwrite the config with the options provided here.
// This is useful for pricing where we may want to use different parameters than the default config.
// With pricing, it is desired to use more pools with lower min liquidity parameter.
type RouterOptions struct {
	MaxPoolsPerRoute int
	MaxRoutes        int
	MaxSplitRoutes   int
	// MinPoolLiquidityCap is the minimum liquidity capitalization required for a pool to be considered in the route.
	MinPoolLiquidityCap uint64
	// The number of milliseconds to cache candidate routes for before expiry.
	CandidateRouteCacheExpirySeconds int
	RankedRouteCacheExpirySeconds    int
	// DisableCache flag controlling whether candidate route and ranked route caches should be disabled.
	// If true, neither of the caches is read or written to.
	DisableCache bool
	// CandidateRoutesPoolFiltersAnyOf are the callbacks that take in a pool, returning
	// true if the candidate route algorithm should ignore a pool matching a certain condition.
	// If at least one of the callbacks in-slice returns true, the ShouldSkipPool function will
	// also return true.
	CandidateRoutesPoolFiltersAnyOf []CandidateRoutePoolFiltrerCb
}

// DefaultRouterOptions defines the default options for the router
var DefaultRouterOptions = RouterOptions{}

// RouterOption configures the router options.
type RouterOption func(*RouterOptions)

// WithMinPoolLiquidityCap configures the router options with the min pool liquidity
// capitalization.
func WithMinPoolLiquidityCap(minPoolLiquidityCap uint64) RouterOption {
	return func(o *RouterOptions) {
		o.MinPoolLiquidityCap = minPoolLiquidityCap
	}
}

// WithMaxPoolsPerRoute configures the router options with the max pools per route.
func WithMaxPoolsPerRoute(maxPoolsPerRoute int) RouterOption {
	return func(o *RouterOptions) {
		o.MaxPoolsPerRoute = maxPoolsPerRoute
	}
}

// WithMaxRoutes configures the router options with the max routes.
func WithMaxRoutes(maxRoutes int) RouterOption {
	return func(o *RouterOptions) {
		o.MaxRoutes = maxRoutes
	}
}

// WithDisableSplitRoutes configures the router options with the disabled split routes.
func WithDisableSplitRoutes() RouterOption {
	return WithMaxSplitRoutes(DisableSplitRoutes)
}

// WithMaxSplitRoutes configures the router options with the max split routes.
func WithMaxSplitRoutes(maxSplitRoutes int) RouterOption {
	return func(o *RouterOptions) {
		o.MaxSplitRoutes = maxSplitRoutes
	}
}

// WithDisableCache configures the options to disable cache.
func WithDisableCache() RouterOption {
	return func(o *RouterOptions) {
		o.DisableCache = true
	}
}

// WithCandidateRoutesPoolFiltersAnyOf configures the router options with the candidate routes pool filters.
// If at least one of the callbacks in-slice returns true, for a specific pool, that pool would be ignored
// in the candidate route search.
func WithCandidateRoutesPoolFiltersAnyOf(filters ...CandidateRoutePoolFiltrerCb) RouterOption {
	return func(o *RouterOptions) {
		o.CandidateRoutesPoolFiltersAnyOf = filters
	}
}

// CandidateRouteSearchDataWorker defines the interface for the candidate route search data worker.
// It pre-computes data necessary for efficiently computing candidate routes.
type CandidateRouteSearchDataWorker interface {
	// ComputeSearchDataSync computes the candidate route search data synchronously.
	ComputeSearchDataSync(ctx context.Context, height uint64, uniqueBlockPoolMetaData BlockPoolMetadata) error

	// ComputeSearchDataAsync computes the candidate route search data asyncronously.
	ComputeSearchDataAsync(ctx context.Context, height uint64, uniqueBlockPoolMetaData BlockPoolMetadata) error

	// RegisterListener registers a listener for candidate route data updates.
	RegisterListener(listener CandidateRouteSearchDataUpdateListener)
}

// PricingUpdateListener defines the interface for the candidate route search data listener.
type CandidateRouteSearchDataUpdateListener interface {
	// OnSearchDataUpdate notifies the listener of the candidate route data update.
	OnSearchDataUpdate(ctx context.Context, height uint64) error
}
