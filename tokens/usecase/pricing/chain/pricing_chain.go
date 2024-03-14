package chainpricing

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type chainPricing struct {
	TUsecase mvc.TokensUsecase
	RUsecase mvc.RouterUsecase

	cache         *cache.Cache
	cacheExpiryNs time.Duration

	maxPoolsPerRoute int
	maxRoutes        int
	minOSMOLiquidity int
}

var _ domain.PricingStrategy = &chainPricing{}

var (
	cacheHitsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_cache_hits_total",
			Help: "Total number of pricing cache hits",
		},
		[]string{"base", "quote"},
	)
	cacheMissesCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_cache_misses_total",
			Help: "Total number of pricing cache misses",
		},
		[]string{"base", "quote"},
	)

	pricesTruncationCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_truncation_total",
			Help: "Total number of price truncations in intermediary calculations",
		},
		[]string{"base", "quote"},
	)

	pricesSpotPriceError = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sqs_pricing_spot_price_error_total",
			Help: "Total number of spot price errors in pricing",
		},
		[]string{"base", "quote"},
	)
)

func init() {
	prometheus.MustRegister(cacheHitsCounter)
	prometheus.MustRegister(cacheMissesCounter)
}

func New(routerUseCase mvc.RouterUsecase, tokenUseCase mvc.TokensUsecase, config domain.PricingConfig) domain.PricingStrategy {
	return &chainPricing{
		RUsecase: routerUseCase,
		TUsecase: tokenUseCase,

		cache:            cache.New(),
		cacheExpiryNs:    time.Duration(config.CacheExpiryMs) * time.Millisecond,
		maxPoolsPerRoute: config.MaxPoolsPerRoute,
		maxRoutes:        config.MaxRoutes,
		minOSMOLiquidity: config.MinOSMOLiquidity,
	}
}

// GetPrice implements pricing.PricingStrategy.
func (c *chainPricing) GetPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// equal base and quote yield the price of one
	if baseDenom == quoteDenom {
		return osmomath.OneBigDec(), nil
	}

	cacheKey := formatCacheKey(baseDenom, quoteDenom)

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

	// Get on-chain scaling factor for base denom.
	baseDenomScalingFactor, err := c.TUsecase.GetChainScalingFactorByDenomMut(ctx, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Get on-chain scaling factor for quote denom.
	quoteDenomScalingFactor, err := c.TUsecase.GetChainScalingFactorByDenomMut(ctx, quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Create a quote denom coin.
	tenQuoteCoin := sdk.NewCoin(quoteDenom, osmomath.NewInt(10).Mul(quoteDenomScalingFactor.TruncateInt()))

	// Overwrite default config with custom values
	// necessary for pricing.
	routerConfig := c.RUsecase.GetConfig()
	routerConfig.MaxRoutes = c.maxRoutes
	routerConfig.MaxPoolsPerRoute = c.maxPoolsPerRoute
	routerConfig.MinOSMOLiquidity = c.minOSMOLiquidity
	// Disable split routes
	routerConfig.MaxSplitIterations = domain.DisableSplitRoutes

	// Compute a quote for one quote coin.
	quote, err := c.RUsecase.GetOptimalQuoteFromConfig(ctx, tenQuoteCoin, baseDenom, routerConfig)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	routes := quote.GetRoute()
	if len(routes) == 0 {
		return osmomath.BigDec{}, fmt.Errorf("no route found when computing pricing for %s (base) -> %s (quote)", baseDenom, quoteDenom)
	}

	route := routes[0]

	chainPrice := osmomath.OneBigDec()

	pools := route.GetPools()

	var (
		tempQuoteDenom       = quoteDenom
		tempBaseDenom        string
		useAlternativeMethod = false
	)

	for _, pool := range pools {
		tempBaseDenom = pool.GetTokenOutDenom()

		// Get spot price for the pool.
		poolSpotPrice, err := c.RUsecase.GetPoolSpotPrice(ctx, pool.GetId(), tempQuoteDenom, tempBaseDenom)
		if err != nil || poolSpotPrice.IsNil() || poolSpotPrice.IsZero() {

			// Increase price truncation counter
			pricesSpotPriceError.WithLabelValues(baseDenom, quoteDenom).Inc()

			useAlternativeMethod = true
			break
		}

		// Multiply spot price by the previous spot price.
		chainPrice = chainPrice.MulMut(poolSpotPrice)

		tempQuoteDenom = tempBaseDenom
	}

	if useAlternativeMethod {
		// Compute on-chain price for 1 unit of base denom and quote denom.
		chainPrice = osmomath.NewBigDecFromBigInt(tenQuoteCoin.Amount.BigIntMut()).QuoMut(osmomath.NewBigDecFromBigInt(quote.GetAmountOut().BigIntMut()))
	} else {
		chainPrice = osmomath.OneBigDec().QuoMut(chainPrice)
	}

	if chainPrice.IsZero() {
		// Increase price truncation counter
		pricesTruncationCounter.WithLabelValues(baseDenom, quoteDenom).Inc()
	}

	// Compute precision scaling factor.
	precisionScalingFactor := osmomath.BigDecFromDec(osmomath.NewDec(10).MulMut(baseDenomScalingFactor.Quo(tenQuoteCoin.Amount.ToLegacyDec())))

	// Apply scaling facors to descale the amounts to real amounts.
	currentPrice := chainPrice.MulMut(precisionScalingFactor)

	// Only store values that are valid.
	if !currentPrice.IsNil() {
		c.cache.Set(cacheKey, currentPrice, c.cacheExpiryNs)
	}

	return currentPrice, nil
}

func formatCacheKey(a, b string) string {
	if a < b {
		a, b = b, a
	}

	var sb strings.Builder
	sb.WriteString(a)
	sb.WriteString(b)
	return sb.String()
}
