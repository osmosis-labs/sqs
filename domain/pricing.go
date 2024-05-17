package domain

import (
	"context"
	"strings"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain/cache"
)

// PricingSourceType defines the enumeration
// for possible pricing sources.
type PricingSourceType int

const (
	// ChainPricingSourceType defines the pricing source
	// by routing through on-chain pools.
	ChainPricingSourceType PricingSourceType = iota
	// CoinGeckoPricingSourceType defines the pricing source
	// that calls CoinGecko API.
	CoinGeckoPricingSourceType
)

// PricingSource defines an interface that must be fulfilled by the specific
// implementation of the pricing source.
type PricingSource interface {
	// GetPrice returns the price given a base and a quote denom or otherwise error, if any.
	// It attempts to find the price from the cache first, and if not found, it will proceed
	// to recomputing it via ComputePrice().
	GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...PricingOption) (osmomath.BigDec, error)

	// InitializeCache initialize the cache for the pricing source to a given value.
	// Panics if cache is already set.
	InitializeCache(*cache.Cache)
}

// DefaultMinLiquidityOption defines the default min liquidity option.
// Per the config file set at start-up
const DefaultMinLiquidityOption = -1

// PricingOptions defines the options for retrieving the prices.
type PricingOptions struct {
	// RecomputePrices defines whether to recompute the prices or attempt to retrieve
	// them from cache first.
	// If set to false, the prices might still be recomputed if the cache is empty.
	RecomputePrices bool
	// RecomputePricesIsSpotPriceComputeMethod defines whether to recompute the prices using the spot price compute method
	// or the quote-based method.
	// For more context, see tokens/usecase/pricing/chain defaultIsSpotPriceComputeMethod.
	RecomputePricesIsSpotPriceComputeMethod bool
	// MinLiquidity defines the minimum liquidity required to consider a pool for pricing.
	MinLiquidity int
}

// DefaultPricingOptions defines the default options for retrieving the prices.
var DefaultPricingOptions = PricingOptions{
	RecomputePrices:                         false,
	MinLiquidity:                            DefaultMinLiquidityOption,
	RecomputePricesIsSpotPriceComputeMethod: true,
}

// PricingOption configures the pricing options.
type PricingOption func(*PricingOptions)

// WithRecomputePrices configures the pricing options to recompute the prices.
func WithRecomputePrices() PricingOption {
	return func(o *PricingOptions) {
		o.RecomputePrices = true
	}
}

// WithRecomputePricesQuoteBasedMethod configures the pricing options to recompute the prices
// using the quote-based method
func WithRecomputePricesQuoteBasedMethod() PricingOption {
	return func(o *PricingOptions) {
		o.RecomputePrices = true
		o.RecomputePricesIsSpotPriceComputeMethod = false
	}
}

// WithMinLiquidity configures the min liquidity option.
func WithMinLiquidity(minLiquidity int) PricingOption {
	return func(o *PricingOptions) {
		// If the min liquidity is the default value, we don't need to set it.
		if minLiquidity == DefaultMinLiquidityOption {
			return
		}

		o.MinLiquidity = minLiquidity
	}
}

// PricingConfig defines the configuration for the pricing.
type PricingConfig struct {
	// The number of milliseconds to cache the pricing data for.
	CacheExpiryMs int `mapstructure:"cache-expiry-ms"`

	// The default quote chain denom.
	DefaultSource PricingSourceType `mapstructure:"default-source"`

	// The default quote chain denom.
	DefaultQuoteHumanDenom string `mapstructure:"default-quote-human-denom"`

	MaxPoolsPerRoute int `mapstructure:"max-pools-per-route"`
	MaxRoutes        int `mapstructure:"max-routes"`
	// Denominated in OSMO (not uosmo)
	MinOSMOLiquidity int `mapstructure:"min-osmo-liquidity"`
}

// FormatCacheKey formats the cache key for the given denoms.
func FormatPricingCacheKey(a, b string) string {
	if a < b {
		a, b = b, a
	}

	var sb strings.Builder
	sb.WriteString(a)
	sb.WriteString(b)
	return sb.String()
}

type PricingWorker interface {
	// UpdatePrices updates prices for the given base denoms asyncronously.
	// Returns a channel that will be closed when the update is completed.
	// Propagates the results to the listeners.
	UpdatePricesAsync(height uint64, baseDenoms map[string]struct{})

	// RegisterListener registers a listener for pricing updates.
	RegisterListener(listener PricingUpdateListener)

	// IsProcessing returns true if the worker is processing a pricing update.
	IsProcessing() bool
}

type PricingUpdateListener interface {
	OnPricingUpdate(ctx context.Context, height int64, pricesBaseQuoteDenomMap map[string]map[string]any, quoteDenom string) error
}
