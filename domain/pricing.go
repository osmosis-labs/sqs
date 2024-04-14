package domain

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
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
	// GetPrice returns the price given a bse and a quote denom or otherwise error, if any.
	GetPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)

	ComputePrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)
}

type PriceRetrievalStrategy struct {
	RecomputePrice bool
}

type PricingOptions struct {
	RecomputePrices   bool
	PricingSourceType PricingSourceType
}

// Option configures the options.
type PricingOption func(*PricingOptions)

func WithRecomputePrices() PricingOption {
	return func(o *PricingOptions) {
		o.RecomputePrices = true
	}
}

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
