package coingeckopricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	cacheHitsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_coingecko_cache_hits_total",
			Help: "Total number of pricing coingecko cache hits",
		},
		[]string{"base", "quote"},
	)
	cacheMissesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_coingecko_cache_misses_total",
			Help: "Total number of pricing coingecko cache misses",
		},
		[]string{"base", "quote"},
	)
)

// CoingeckoPriceGetterFn is a function type that fetches the price of a token from Coingecko.
// We monkey-patch this function for testing purposes.
type CoingeckoPriceGetterFn func(ctx context.Context, baseDenom string, coingeckoId string) (osmomath.BigDec, error)

// DefaultCoingeckoPriceGetter is the default implementation of CoingeckoPriceGetterFn.
var DefaultCoingeckoPriceGetter CoingeckoPriceGetterFn = nil

type coingeckoPricing struct {
	TUsecase      mvc.TokensUsecase
	cache         *cache.Cache
	cacheExpiryNs time.Duration
	quoteCurrency string
	coingeckoUrl  string

	// We monkey-patch this function for testing purposes.
	priceGetterFn CoingeckoPriceGetterFn
}

func init() {
	if err := prometheus.Register(cacheHitsCounter); err != nil {
		panic(err)
	}
	if err := prometheus.Register(cacheMissesCounter); err != nil {
		panic(err)
	}
}

// New creates a new Coingecko pricing source.
// if coinGeckoPriceGetterFn is nil, it uses the default implementation.
func New(routerUseCase mvc.RouterUsecase, tokenUseCase mvc.TokensUsecase, config domain.PricingConfig, coingeckoPriceGetterFn CoingeckoPriceGetterFn) domain.PricingSource {
	coingeckoPricing := &coingeckoPricing{
		TUsecase:      tokenUseCase,
		cache:         cache.New(),
		cacheExpiryNs: time.Duration(config.CacheExpiryMs) * time.Millisecond,
		quoteCurrency: config.CoingeckoQuoteCurrency,
		coingeckoUrl:  config.CoingeckoUrl,
	}

	if coingeckoPriceGetterFn == nil {
		// Set the default price getter function.
		coingeckoPricing.priceGetterFn = coingeckoPricing.GetPriceByCoingeckoId
	} else {
		// Set the custom price getter function (useful for testing purposes)
		coingeckoPricing.priceGetterFn = coingeckoPriceGetterFn
	}

	return coingeckoPricing
}

// GetPrice implements pricing.PricingStrategy.
// quoteDenom is ignored as it uses always coingecko-quote-currency in config.json
func (c *coingeckoPricing) GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...domain.PricingOption) (osmomath.BigDec, error) {
	coingeckoId, err := c.TUsecase.GetCoingeckoIdByChainDenom(baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	cacheKey := domain.FormatPricingCacheKey(baseDenom, c.quoteCurrency)
	cachedValue, found := c.cache.Get(cacheKey)

	if found {
		// Cast cached value to correct type.
		cachedBigDecPrice, ok := cachedValue.(osmomath.BigDec)
		if !ok {
			return osmomath.BigDec{}, fmt.Errorf("invalid type cached in pricing, expected BigDec, got (%T)", cachedValue)
		}
		// Increase cache hits
		cacheHitsCounter.WithLabelValues(baseDenom, quoteDenom).Inc()
		return cachedBigDecPrice, nil
	} else if !found {
		// Increase cache misses
		cacheMissesCounter.WithLabelValues(baseDenom, quoteDenom).Inc()
	}

	price, err := c.GetPriceByCoingeckoId(ctx, baseDenom, coingeckoId)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	return price, nil
}

// GetPriceByCoingeckoId fetches the price of a token from Coingecko.
func (c coingeckoPricing) GetPriceByCoingeckoId(ctx context.Context, baseDenom string, coingeckoId string) (osmomath.BigDec, error) {
	url := fmt.Sprintf("%s?ids=%s&vs_currencies=%s", c.coingeckoUrl, coingeckoId, c.quoteCurrency)
	resp, err := http.Get(url)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return osmomath.BigDec{}, fmt.Errorf("failed to get price from Coingecko: %s", resp.Status)
	}

	var data map[string]map[string]float64
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return osmomath.BigDec{}, fmt.Errorf("failed to decode Coingecko response: %s", err)
	}

	price, ok := data[coingeckoId][c.quoteCurrency]
	if !ok {
		return osmomath.BigDec{}, fmt.Errorf("price not found for coingecko ID: %s", coingeckoId)
	}

	result, err := osmomath.NewBigDecFromStr(fmt.Sprintf("%f", price))
	if err != nil {
		return osmomath.BigDec{}, err
	}

	cacheKey := domain.FormatPricingCacheKey(baseDenom, c.quoteCurrency)
	c.cache.Set(cacheKey, result, c.cacheExpiryNs)

	return result, nil
}

// InitializeCache implements pricing.PricingSource
func (c *coingeckoPricing) InitializeCache(cache *cache.Cache) {
	c.cache = cache
}

// GetFallbackStrategy implements pricing.PricingSource
func (c *coingeckoPricing) GetFallbackStrategy(quoteDenom string) domain.PricingSourceType {
	// Currently there is no fallback mechanism for Coingecko
	return domain.NoneSourceType
}
