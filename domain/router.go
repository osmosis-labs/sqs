package domain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v21/x/poolmanager/types"
)

type RoutablePool interface {
	GetId() uint64

	GetType() poolmanagertypes.PoolType

	GetPoolDenoms() []string

	GetTokenOutDenom() string

	CalcSpotPrice(baseDenom string, quoteDenom string) (osmomath.BigDec, error)

	CalculateTokenOutByTokenIn(tokenIn sdk.Coin) (sdk.Coin, error)
	ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin)

	// SetTokenOutDenom sets the token out denom on the routable pool.
	SetTokenOutDenom(tokenOutDenom string)

	GetTakerFee() osmomath.Dec

	GetSpreadFactor() osmomath.Dec

	String() string
}

type RoutableResultPool interface {
	RoutablePool
	GetBalances() sdk.Coins
}

type Route interface {
	GetPools() []RoutablePool
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
	PreferredPoolIDs   []uint64 `mapstructure:"preferred_pool_ids"`
	MaxPoolsPerRoute   int      `mapstructure:"max_pools_per_route"`
	MaxRoutes          int      `mapstructure:"max_routes"`
	MaxSplitRoutes     int      `mapstructure:"max_split_routes"`
	MaxSplitIterations int      `mapstructure:"max_split_iterations"`
	// Denominated in OSMO (not uosmo)
	MinOSMOLiquidity          int  `mapstructure:"min_osmo_liquidity"`
	RouteUpdateHeightInterval int  `mapstructure:"route_update_height_interval"`
	RouteCacheEnabled         bool `mapstructure:"route_cache_enabled"`
	// The number of seconds to cache routes for before expiry.
	RouteCacheExpirySeconds uint64 `mapstructure:"route_cache_expiry_seconds"`
}
