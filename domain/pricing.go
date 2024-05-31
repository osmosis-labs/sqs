package domain

import (
	"context"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
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
	NoneSourceType = -1
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

	// GetFallBackStrategy determines what pricing source should be fallen back to in case this pricing source fails
	GetFallbackStrategy(quoteDenom string) PricingSourceType
}

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
	// MinPoolLiquidityCap defines the minimum liquidity required to consider a pool for pricing.
	MinPoolLiquidityCap uint64
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

// WithMinPricingPoolLiquidityCap configures the min liquidity capitalization option
// for pricing. Note, that non-pricing routing has its own RouterOption to configure
// the min liquidity capitalization.
func WithMinPricingPoolLiquidityCap(minPoolLiquidityCap uint64) PricingOption {
	return func(o *PricingOptions) {
		o.MinPoolLiquidityCap = minPoolLiquidityCap
	}
}

// PricingConfig defines the configuration for the pricing.
type PricingConfig struct {
	// The number of milliseconds to cache the pricing data for.
	CacheExpiryMs int `mapstructure:"cache-expiry-ms"`

	// The default quote chain denom.
	// 0 stands for chain. 1 for Coingecko.
	DefaultSource PricingSourceType `mapstructure:"default-source"`

	// The default quote chain denom.
	DefaultQuoteHumanDenom string `mapstructure:"default-quote-human-denom"`

	// Coingecko URL endpoint.
	CoingeckoUrl string `mapstructure:"coingecko-url"`

	// Coingecko quote currency for fetching prices.
	CoingeckoQuoteCurrency string `mapstructure:"coingecko-quote-currency"`

	MaxPoolsPerRoute int `mapstructure:"max-pools-per-route"`
	MaxRoutes        int `mapstructure:"max-routes"`
	// MinPoolLiquidityCap is the minimum liquidity capitalization required for a pool to be considered in the router.
	MinPoolLiquidityCap uint64 `mapstructure:"min-pool-liquidity-cap"`
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
	// UpdatePrices updates prices for the tokens from the unique block pool metadata
	// that contains information about changed denoms and pools within a block.
	// Propagates the results to the listeners.
	UpdatePricesAsync(height uint64, uniqueBlockPoolMetaData BlockPoolMetadata)

	// RegisterListener registers a listener for pricing updates.
	RegisterListener(listener PricingUpdateListener)
}

type PricingUpdateListener interface {
	OnPricingUpdate(ctx context.Context, height int64, blockMetaData BlockPoolMetadata, pricesBaseQuoteDenomMap PricesResult, quoteDenom string) error
}

type DenomPriceInfo struct {
	Price         osmomath.BigDec
	ScalingFactor osmomath.Dec
}

type LiquidityPricer interface {
	// ComputeCoinCap computes the equivalent of the given coin in the desired quote denom that is set on ingester.
	// Returns error if:
	// * Price is zero
	// * Scaling factor is zero
	// * Truncation occurs in intermediary operations. Truncation is defined as the original amount
	// being non-zero and the computed amount being zero.
	ComputeCoinCap(coin sdk.Coin, baseDenomPriceData DenomPriceInfo) (osmomath.Dec, error)
}

// PoolLiquidityComputeListener defines the interface for the pool liquidity compute listener.
// It is used to notify the listeners of the pool liquidity compute worker that the computation
// for a given height is completed.
type PoolLiquidityComputeListener interface {
	OnPoolLiquidityCompute(height int64, updatedPoolIDs []uint64) error
}

// PricesResult defines the result of the prices.
// [base denom][quote denom] => price
// Note: BREAKING API - this type is API breaking as it is serialized to JSON.
// from the /tokens/prices endpoint. Be mindful of changing it without
// separating the API response for backward compatibility.
type PricesResult map[string]map[string]osmomath.BigDec
