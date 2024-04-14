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
	GetPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)

	// ComputePrice computes the price given a base and a quote denom or otherwise error, if any.
	// Writes the computed price to the cach according to the configured TVL at the time of strategy creation.
	ComputePrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)

	// InitializeCache initialize the cache for the pricing source to a given value.
	// Panics if cache is already set.
	InitializeCache(*cache.Cache)
}

// PricingOptions defines the options for retrieving the prices.
type PricingOptions struct {
	// RecomputePrices defines whether to recompute the prices or attempt to retrieve
	// them from cache first.
	// If set to false, the prices might still be recomputed if the cache is empty.
	RecomputePrices bool
	// PricingSourceType defines the source of the pricing.
	PricingSourceType PricingSourceType
}

// DefaultPricingOptions defines the default options for retrieving the prices.
var DefaultPricingOptions = PricingOptions{
	RecomputePrices:   false,
	PricingSourceType: ChainPricingSourceType,
}

// PricingOption configures the pricing options.
type PricingOption func(*PricingOptions)

// WithRecomputePrices configures the pricing options to recompute the prices.
func WithRecomputePrices() PricingOption {
	return func(o *PricingOptions) {
		o.RecomputePrices = true
	}
}

// WithPricingSource configures the pricing options to use the specified pricing source.
func WithPricingSource(pricingSource PricingSourceType) PricingOption {
	return func(o *PricingOptions) {
		o.PricingSourceType = pricingSource
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
