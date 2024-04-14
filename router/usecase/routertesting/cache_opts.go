package routertesting

import "github.com/osmosis-labs/sqs/domain/cache"

// CacheOptions is a struct that holds the cache options for the router testing.
type CacheOptions struct {
	CandidateRoutes *cache.Cache
	RankedRoutes    *cache.Cache
	Pricing         *cache.Cache
}

// CacheOption is a function that sets the cache options for the router testing.
type CacheOption func(*CacheOptions)

// WithCandidateRoutesCache sets the cache for candidate routes.
func WithCandidateRoutesCache(cache *cache.Cache) CacheOption {
	return func(options *CacheOptions) {
		options.CandidateRoutes = cache
	}
}

// WithRankedRoutesCache sets the cache for ranked routes.
func WithRankedRoutesCache(cache *cache.Cache) CacheOption {
	return func(options *CacheOptions) {
		options.RankedRoutes = cache
	}
}

// WithPricingCache sets the cache for pricing.
func WithPricingCache(cache *cache.Cache) CacheOption {
	return func(options *CacheOptions) {
		options.Pricing = cache
	}
}
