package chainpricing

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type chainPricing struct {
	TUsecase mvc.TokensUsecase
	RUsecase mvc.SimpleRouterUsecase

	cache         *cache.Cache
	cacheExpiryNs time.Duration
	// failedCache holds the error of a recent failed price computation per pair, so that a pair
	// without a usable route is not searched again on every request. It is separate from cache
	// so that a failure still evicts the last good price there. It uses the same key as cache:
	// a price computed at one min pool liquidity cap serves the others, and so does a failure,
	// which lets the pricing worker's failures answer requests too. Each direction of a pair has
	// its own key, since one price is the inverse of the other.
	failedCache *cache.Cache
	// pairShards hold the ordering state of the pairs with a computation in flight, so that a failure
	// that started before a newer success of its pair does not evict that success's price or replace it
	// in failedCache. Each shard's lock also makes a computation's final cache update atomic per pair.
	// Pairs are spread over the shards by key hash.
	pairShards [pairShardCount]pairShard

	defaultQuoteDenom string

	maxPoolsPerRoute    int
	maxRoutes           int
	minPoolLiquidityCap uint64
	// workerMinPoolLiquidityCap is the min pool liquidity cap the pricing worker uses for the default quote denom.
	// Request-path recomputes of default quote prices use it too, so that a price refreshed after expiry
	// is computed the same way as one precomputed by the worker.
	workerMinPoolLiquidityCap uint64
}

var _ domain.PricingSource = &chainPricing{}

const (
	// We use multiplier so that stablecoin quotes avoid selecting low liquidity routes.
	// USDC/USDT value of 10 should be sufficient to avoid low liquidity routes.
	tokenInMultiplier = 10

	// SpotPriceComputeMethod is used to compute the price using spot prices
	// over routes between 2 tokens.
	// The is the default method used to compute prices.
	//
	// QuoteBasedComputeMethod is used to compute the price using a quote
	// for 10 unit of the token in.
	// This is a fallback method used when spot prices are not available due to error.
	//
	// The default is SpotPriceComputeMethod.
	defaultIsSpotPriceComputeMethod bool = true

	// defaultQuotePriceMaxAge bounds how long a default quote denom price is served without being recomputed.
	// The pricing worker only reprices denoms in pools touched by a block, so without a bound a price
	// derived through an intermediate denom (e.g. TOKEN/OSMO then OSMO/USDC) would never refresh.
	defaultQuotePriceMaxAge = 10 * time.Minute

	// failedPriceCacheExpiry is how long a failed price computation is served from failedCache
	// before the price is computed again. Recomputes requested by the pricing worker ignore it.
	failedPriceCacheExpiry = 10 * time.Second

	// pairShardCount is the number of shards pairs are spread over for their ordering state and final cache update.
	pairShardCount = 64
)

func New(routerUseCase mvc.SimpleRouterUsecase, tokenUseCase mvc.TokensUsecase, config domain.PricingConfig) domain.PricingSource {
	chainDefaultHumanDenom, err := tokenUseCase.GetChainDenom(config.DefaultQuoteHumanDenom)
	if err != nil {
		panic(fmt.Sprintf("failed to get chain denom for default quote human denom (%s): %s", config.DefaultQuoteHumanDenom, err))
	}

	return &chainPricing{
		RUsecase: routerUseCase,
		TUsecase: tokenUseCase,

		cache:                     cache.New("chain_prices"),
		failedCache:               cache.New("chain_failed_prices"),
		cacheExpiryNs:             time.Duration(config.CacheExpiryMs) * time.Millisecond,
		maxPoolsPerRoute:          config.MaxPoolsPerRoute,
		maxRoutes:                 config.MaxRoutes,
		minPoolLiquidityCap:       config.MinPoolLiquidityCap,
		workerMinPoolLiquidityCap: config.WorkerMinPoolLiquidityCap,
		defaultQuoteDenom:         chainDefaultHumanDenom,
	}
}

// GetPrice implements pricing.PricingStrategy.
func (c *chainPricing) GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	options := domain.PricingOptions{
		MinPoolLiquidityCap:                     c.minPoolLiquidityCap,
		RecomputePricesIsSpotPriceComputeMethod: defaultIsSpotPriceComputeMethod,
		RecomputePrices:                         false,
	}

	if quoteDenom == c.defaultQuoteDenom {
		options.MinPoolLiquidityCap = c.workerMinPoolLiquidityCap
	}

	for _, opt := range opts {
		opt(&options)
	}

	// Recompute prices if desired by configuration.
	// Otherwise, look into cache first.
	if options.RecomputePrices {
		return c.computePrice(ctx, baseDenom, quoteDenom, options.MinPoolLiquidityCap, options.RecomputePricesIsSpotPriceComputeMethod)
	}

	// equal base and quote yield the price of one
	if baseDenom == quoteDenom {
		return osmomath.OneBigDec(), nil, nil
	}

	cacheKey := domain.FormatPricingCacheKey(baseDenom, quoteDenom)

	cachedValue, found := c.cache.Get(cacheKey)
	if found {
		// Cast cached value to correct type.
		cachedPrice, ok := cachedValue.(domain.PriceResult)
		if !ok {
			return osmomath.BigDec{}, nil, fmt.Errorf("invalid type cached in pricing, expected BigDec, got (%T)", cachedValue)
		}

		// Increase cache hits
		domain.SQSPricingCacheHitsCounter.Inc()
		return cachedPrice.Price, cachedPrice.Routes, nil
	} else if !found {
		// Increase cache misses
		domain.SQSPricingCacheMissesCounter.Inc()
	}

	// A recent failure is returned without searching for a route again.
	if failedValue, found := c.failedCache.Get(cacheKey); found {
		if failedErr, ok := failedValue.(error); ok {
			return osmomath.BigDec{}, nil, failedErr
		}
	}

	// If cache miss occurs, we compute the price.
	return c.computePrice(ctx, baseDenom, quoteDenom, options.MinPoolLiquidityCap, options.RecomputePricesIsSpotPriceComputeMethod)
}

// pairShard holds the ordering state of the pairs whose keys hash to it and that have a
// computation in flight. A pair's state lives exactly as long as one of its computations runs, so
// the shard holds at most one entry per concurrent computation and none is ever evicted early.
type pairShard struct {
	mu    sync.Mutex
	pairs map[string]*pairOrder
}

// pairOrder is the ordering state of one pair while at least one of its computations runs.
type pairOrder struct {
	// inFlight is the number of computations of the pair that have begun and not yet finished.
	inFlight int
	// successes counts the successful computations of the pair since its state was created.
	successes uint64
}

// begin records the start of a computation of the pair under key and returns the pair's success
// count, which the computation passes back to finishLocked.
func (s *pairShard) begin(key string) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.pairs == nil {
		s.pairs = make(map[string]*pairOrder)
	}
	order, ok := s.pairs[key]
	if !ok {
		order = &pairOrder{}
		s.pairs[key] = order
	}
	order.inFlight++
	return order.successes
}

// finishLocked records the end of a computation of the pair under key that began with
// startSuccesses successes, and reports whether another computation of the pair succeeded since.
// CONTRACT: s.mu is held, and begin was called for key by the finishing computation.
func (s *pairShard) finishLocked(key string, startSuccesses uint64, succeeded bool) (succeededSince bool) {
	order, ok := s.pairs[key]
	if !ok || order.inFlight <= 0 {
		panic(fmt.Sprintf("pricing pair %s finished a computation it did not begin", key))
	}

	succeededSince = order.successes != startSuccesses
	if succeeded {
		order.successes++
	}
	order.inFlight--
	if order.inFlight == 0 {
		delete(s.pairs, key)
	}
	return succeededSince
}

// pairShard returns the shard that holds the ordering state of the pair under cacheKey.
func (c *chainPricing) pairShard(cacheKey string) *pairShard {
	h := fnv.New32a()
	_, _ = h.Write([]byte(cacheKey))
	return &c.pairShards[h.Sum32()%pairShardCount]
}

// computePrice computes the price for a given base and quote denom
// On failure, any previously cached price for the pair is evicted so that a stale price is not
// served after its route stops being usable, and fallback pricing can take over.
// The failure is kept in failedCache for failedPriceCacheExpiry. A success clears it.
// A failure that started before a newer success for the pair, or that came from the request's
// context being cancelled or timing out, which says nothing about the pair, does neither.
func (c *chainPricing) computePrice(ctx context.Context, baseDenom string, quoteDenom string, minPoolLiquidityCap uint64, isSpotPriceComputeMethod bool) (_ osmomath.BigDec, _ []domain.SplitRoute, err error) {
	cacheKey := domain.FormatPricingCacheKey(baseDenom, quoteDenom)
	shard := c.pairShard(cacheKey)
	startSuccesses := shard.begin(cacheKey)
	// The computed price is published to cache by the deferred update, under the pair's shard lock, so
	// that an older failure cannot evict it between its publication and the record of its success.
	var computed *domain.PriceResult
	var computedTTL time.Duration

	defer func() {
		// A panic (e.g. from a malformed pool spot price) must evict too, so it is turned into an error.
		if r := recover(); r != nil {
			err = fmt.Errorf("panic computing price for %s (base) -> %s (quote): %v", baseDenom, quoteDenom, r)
		}
		shard.mu.Lock()
		defer shard.mu.Unlock()
		succeededSince := shard.finishLocked(cacheKey, startSuccesses, err == nil)
		if err != nil {
			// A newer success already holds the pair's current price, and a computation cut short
			// by its context says nothing about the pair.
			if succeededSince || ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			c.cache.Delete(cacheKey)
			c.failedCache.Set(cacheKey, err, failedPriceCacheExpiry)
		} else {
			if computed != nil {
				c.cache.Set(cacheKey, *computed, computedTTL)
			}
			c.failedCache.Delete(cacheKey)
		}
	}()

	if baseDenom == quoteDenom {
		return osmomath.OneBigDec(), nil, nil
	}

	// Get on-chain scaling factor for base denom.
	baseDenomScalingFactor, err := c.TUsecase.GetChainScalingFactorByDenomMut(baseDenom)
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}

	// Get on-chain scaling factor for quote denom.
	quoteDenomScalingFactor, err := c.TUsecase.GetChainScalingFactorByDenomMut(quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}

	// Create a quote denom coin.
	// We use multiplier so that stablecoin quotes avoid selecting low liquidity routes.
	tenQuoteCoin := sdk.NewCoin(quoteDenom, osmomath.NewInt(tokenInMultiplier).Mul(quoteDenomScalingFactor.TruncateInt()))

	// Overwrite default config with custom values
	// necessary for pricing.
	routingOptions := []domain.RouterOption{
		domain.WithMaxRoutes(c.maxRoutes),
		domain.WithMaxPoolsPerRoute(c.maxPoolsPerRoute),
		// Use the provided min liquidity value rather than the default
		// Since it can be overridden by options in GetPrice(...)
		domain.WithMinPoolLiquidityCap(minPoolLiquidityCap),
		domain.WithDisableSplitRoutes(),
		domain.WithCandidateRoutesPoolFiltersAnyOf(domain.ShouldSkipOrderbookPool),
	}

	// Compute a quote for one quote coin.
	quote, routes, err := c.RUsecase.GetSimpleQuote(ctx, tenQuoteCoin, baseDenom, routingOptions...)
	if errors.Is(err, domain.ErrNoCandidateRoutes) {
		return osmomath.BigDec{}, nil, domain.NoPriceError{Err: fmt.Errorf("no route found when computing pricing for %s (base) -> %s (quote): %w", baseDenom, quoteDenom, err)}
	}
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}
	if quote == nil {
		return osmomath.BigDec{}, nil, domain.NoPriceError{Err: fmt.Errorf("no quote found when computing pricing for %s (base) -> %s (quote)", baseDenom, quoteDenom)}
	}

	if len(routes) == 0 {
		return osmomath.BigDec{}, nil, domain.NoPriceError{Err: fmt.Errorf("no route found when computing pricing for %s (base) -> %s (quote)", baseDenom, quoteDenom)}
	}

	route := routes[0]

	chainPrice := osmomath.OneBigDec()

	pools := route.GetPools()

	var (
		tempQuoteDenom = quoteDenom
		tempBaseDenom  string
	)

	// If we are using spot price method, we compute the result using spot-prices over
	// pools in the quote.
	//
	// We fallback to quote-based compute method if there is an error in spot price computation.
	if isSpotPriceComputeMethod {
		for _, pool := range pools {
			tempBaseDenom = pool.GetTokenOutDenom()

			// Get spot price for the pool.
			poolSpotPrice, err := c.RUsecase.GetPoolSpotPrice(ctx, pool.GetId(), tempQuoteDenom, tempBaseDenom)
			if err != nil || poolSpotPrice.IsNil() || poolSpotPrice.IsZero() {
				// Increase price truncation counter
				domain.SQSPricingSpotPriceError.Inc()

				// Error in spot price, use quote-based compute method.
				isSpotPriceComputeMethod = false
				break
			}

			// Multiply spot price by the previous spot price.
			chainPrice = chainPrice.MulMut(poolSpotPrice)

			tempQuoteDenom = tempBaseDenom
		}
	}

	// This is a separate logic gate to fallback to quote-based compute method
	// if there is an error in the spot price computation above.
	if !isSpotPriceComputeMethod {
		// Compute on-chain price for 10 units of base denom and resulted quote denom out.
		chainPrice = osmomath.NewBigDecFromBigInt(tenQuoteCoin.Amount.BigIntMut()).QuoMut(osmomath.NewBigDecFromBigInt(quote.GetAmountOut().Amount.BigIntMut()))
	}

	if chainPrice.IsZero() {
		// Increase price truncation counter
		domain.SQSPricingTruncationCounter.Inc()
	}

	// Compute precision scaling factor.
	precisionScalingFactor := osmomath.BigDecFromDec(osmomath.NewDec(tokenInMultiplier).MulMut(baseDenomScalingFactor.Quo(tenQuoteCoin.Amount.ToLegacyDec())))

	// Apply scaling facors to descale the amounts to real amounts.
	chainPrice = chainPrice.MulMut(precisionScalingFactor)

	// A truncated or otherwise non-positive price is not a valid price.
	if chainPrice.IsNil() || !chainPrice.IsPositive() {
		return osmomath.BigDec{}, nil, domain.NoPriceError{Err: fmt.Errorf("non-positive price computed for %s (base) -> %s (quote)", baseDenom, quoteDenom)}
	}

	expirationTTL := c.cacheExpiryNs
	// We pre-compute the price for the default quote denom in ingest handler via the background
	// pricing worker, which only updates prices for tokens modified within the block.
	// The max age makes sure that prices for tokens whose own pools are idle are still recomputed.
	if quoteDenom == c.defaultQuoteDenom {
		expirationTTL = defaultQuotePriceMaxAge
	}
	result := domain.NewPriceResult(chainPrice, routes)
	computed, computedTTL = &result, expirationTTL

	return chainPrice, routes, nil
}

// InitializeCache implements domain.PricingSource.
func (c *chainPricing) InitializeCache(cache *cache.Cache) {
	c.cache = cache
}

// GetFallbackStrategy implements pricing.PricingSource
func (c *chainPricing) GetFallbackStrategy(quoteDenom string) domain.PricingSourceType {
	if quoteDenom == c.defaultQuoteDenom {
		return domain.CoinGeckoPricingSourceType
	} else {
		return domain.NoneSourceType
	}
}
