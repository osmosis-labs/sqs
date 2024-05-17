package routertesting

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
)

// MainnetTestOptions is a struct that holds the test options for the suite from mainnet state.
// It is used to set the cache options for the router testing
// and the router config for the router testing
// and the pricing config for the router testing.
type MainnetTestOptions struct {
	CandidateRoutes *cache.Cache
	RankedRoutes    *cache.Cache
	Pricing         *cache.Cache
	RouterConfig    domain.RouterConfig
	PricingConfig   domain.PricingConfig
	PoolsConfig     domain.PoolsConfig
}

// MainnetTestOption is a function that sets the cache options for the router testing.
type MainnetTestOption func(*MainnetTestOptions)

// WithCandidateRoutesCache sets the cache for candidate routes.
func WithCandidateRoutesCache(cache *cache.Cache) MainnetTestOption {
	return func(options *MainnetTestOptions) {
		options.CandidateRoutes = cache
	}
}

// WithRankedRoutesCache sets the cache for ranked routes.
func WithRankedRoutesCache(cache *cache.Cache) MainnetTestOption {
	return func(options *MainnetTestOptions) {
		options.RankedRoutes = cache
	}
}

// WithPricingCache sets the cache for pricing.
func WithPricingCache(cache *cache.Cache) MainnetTestOption {
	return func(options *MainnetTestOptions) {
		options.Pricing = cache
	}
}

// WithRouterConfig sets the router config on options.
func WithRouterConfig(config domain.RouterConfig) MainnetTestOption {
	return func(options *MainnetTestOptions) {
		options.RouterConfig = config
	}
}

// WithPricingConfig sets the pricing config on options.
func WithPricingConfig(config domain.PricingConfig) MainnetTestOption {
	return func(options *MainnetTestOptions) {
		options.PricingConfig = config
	}
}

// WithPoolsConfig sets the pools config on options.
func WithPoolsConfig(config domain.PoolsConfig) MainnetTestOption {
	return func(options *MainnetTestOptions) {
		options.PoolsConfig = config
	}
}
