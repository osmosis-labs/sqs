package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type RoutableResultPool interface {
	sqsdomain.RoutablePool
	GetBalances() sdk.Coins
}

type Route interface {
	GetPools() []sqsdomain.RoutablePool
	// AddPool adds pool to route.
	AddPool(pool sqsdomain.PoolI, tokenOut string, takerFee osmomath.Dec)
	// CalculateTokenOutByTokenIn calculates the token out amount given the token in amount.
	// Returns error if the calculation fails.
	CalculateTokenOutByTokenIn(tokenIn sdk.Coin) (sdk.Coin, error)

	GetTokenOutDenom() string

	// PrepareResultPools strips away unnecessary fields
	// from each pool in the route,
	// leaving only the data needed by client
	// Runs the quote logic one final time to compute the effective spot price.
	// Note that it mutates the route.
	// Computes the spot price of the route.
	// Returns the spot price before swap and effective spot price.
	PrepareResultPools(tokenIn sdk.Coin) (osmomath.Dec, osmomath.Dec, error)

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
	GetEffectiveSpreadFactor() osmomath.Dec
	GetPriceImpact() osmomath.Dec

	// PrepareResult mutates the quote to prepare
	// it with the data formatted for output to the client.
	PrepareResult() ([]SplitRoute, osmomath.Dec)

	String() string
}

type RouterConfig struct {
	PreferredPoolIDs   []uint64 `mapstructure:"preferred-pool-ids"`
	MaxPoolsPerRoute   int      `mapstructure:"max-pools-per-route"`
	MaxRoutes          int      `mapstructure:"max-routes"`
	MaxSplitRoutes     int      `mapstructure:"max-split-routes"`
	MaxSplitIterations int      `mapstructure:"max-split-iterations"`
	// Denominated in OSMO (not uosmo)
	MinOSMOLiquidity          int  `mapstructure:"min-osmo-liquidity"`
	RouteUpdateHeightInterval int  `mapstructure:"route-update-height-interval"`
	RouteCacheEnabled         bool `mapstructure:"route-cache-enabled"`
	// The number of seconds to cache routes for before expiry.
	RouteCacheExpirySeconds uint64 `mapstructure:"route-cache-expiry-seconds"`
	// Flag indicating whether we should have a cache for overwrite routes enabled.
	EnableOverwriteRoutesCache bool `mapstructure:"enable-overwrite-routes-cache"`
}
