package coingeckopricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	sqshttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

const (
	// USDC_DENOM is the alloyed USDC (allUSDC), the default quote denom for SQS prices.
	USDC_DENOM = "factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC"
	// NOBLE_USDC_DENOM is the Noble USDC variant, the previous default quote denom.
	// It remains an accepted quote denom so that callers still quoting in Noble USDC keep working.
	NOBLE_USDC_DENOM = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	USDT_DENOM       = "ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB"
)

const (
	// batchWindow is how long concurrent price requests are collected before one request is sent for all of them.
	batchWindow = 25 * time.Millisecond
	// maxIdsPerRequest caps the number of Coingecko ids in a single request.
	maxIdsPerRequest = 100
	// defaultRefreshInterval is how often tracked ids are re-fetched in bulk when the config leaves it unset.
	defaultRefreshInterval = 5 * time.Minute
	// noPriceCacheTTL is how long an id Coingecko has no price for is answered as unpriced without a request.
	noPriceCacheTTL = 10 * time.Minute
	// trackWindow is how long an id keeps being refreshed after its last lookup.
	trackWindow = time.Hour
	// defaultRateLimitBackoff is used on HTTP 429 when Retry-After is missing or unparsable.
	defaultRateLimitBackoff = 10 * time.Second
	// maxRateLimitBackoff caps the Retry-After delay honoured on HTTP 429.
	maxRateLimitBackoff = 30 * time.Second
)

// CoingeckoPriceGetterFn is a function type that fetches the price of a token from Coingecko.
// We monkey-patch this function for testing purposes.
type CoingeckoPriceGetterFn func(ctx context.Context, baseDenom string, coingeckoId string) (osmomath.BigDec, error)

// DefaultCoingeckoPriceGetter represents a placeholder for the default implementation of CoingeckoPriceGetterFn, which invokes GetPriceByCoingeckoId method that is defined on coingeckoPricing.
var DefaultCoingeckoPriceGetter CoingeckoPriceGetterFn = nil

type coingeckoPricing struct {
	TUsecase      mvc.TokensUsecase
	cache         *cache.Cache
	cacheExpiryNs time.Duration
	quoteCurrency string
	coingeckoUrl  string
	// defaultQuoteDenom is the chain denom resolved from config.DefaultQuoteHumanDenom.
	// Chain pricing falls back to Coingecko only for this denom, so it must always be accepted
	// as a quote denom, even after the human denom is remapped to a different chain denom.
	defaultQuoteDenom string

	// We monkey-patch this function for testing purposes.
	priceGetterFn CoingeckoPriceGetterFn

	// batcher coalesces Coingecko requests made by the default price getter.
	batcher *priceBatcher
}

// New creates a new Coingecko pricing source.
// if coinGeckoPriceGetterFn is nil, it uses the default implementation.
func New(tokenUseCase mvc.TokensUsecase, config domain.PricingConfig, coingeckoPriceGetterFn CoingeckoPriceGetterFn) domain.PricingSource {
	coingeckoPricing := &coingeckoPricing{
		TUsecase:      tokenUseCase,
		cache:         cache.New(),
		cacheExpiryNs: time.Duration(config.CacheExpiryMs) * time.Millisecond,
		quoteCurrency: config.CoingeckoQuoteCurrency,
		coingeckoUrl:  config.CoingeckoUrl,
		batcher:       newPriceBatcher(config.CoingeckoUrl, config.CoingeckoQuoteCurrency, refreshInterval(config)),
	}

	// If the default quote human denom does not resolve, only the stablecoin constants are accepted.
	// The chain pricing source fails at startup on the same lookup, so this is not reached in practice.
	if defaultQuoteDenom, err := tokenUseCase.GetChainDenom(config.DefaultQuoteHumanDenom); err == nil {
		coingeckoPricing.defaultQuoteDenom = defaultQuoteDenom
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
// Coingecko pricing is always usd (i.e. usdc or usdt), as specified in the coingecko-quote-currency in config.json
// So quoteDenom has to be nil, the configured default quote denom, or a USD stablecoin: allUSDC, Noble USDC or USDT.
func (c *coingeckoPricing) GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	if !c.isAllowedQuoteDenom(quoteDenom) {
		return osmomath.BigDec{}, nil, fmt.Errorf("only the default quote denom, usdc/usdt denom or nil is allowed for the quote denom param")
	}
	coingeckoId, err := c.TUsecase.GetCoingeckoIdByChainDenom(baseDenom)
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}

	cacheKey := domain.FormatPricingCacheKey(baseDenom, c.quoteCurrency)
	cachedValue, found := c.cache.Get(cacheKey)

	if found {
		// Cast cached value to correct type.
		cachedBigDecPrice, ok := cachedValue.(osmomath.BigDec)
		if !ok {
			return osmomath.BigDec{}, nil, fmt.Errorf("invalid type cached in pricing, expected BigDec, got (%T)", cachedValue)
		}
		// Increase cache hits
		domain.SQSPricingCoingeckoCacheHitsCounter.Inc()
		return cachedBigDecPrice, nil, nil
	} else if !found {
		// Increase cache misses
		domain.SQSPricingCoingeckoCacheMissesCounter.Inc()
	}

	price, err := c.priceGetterFn(ctx, baseDenom, coingeckoId)
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}
	return price, nil, nil
}

// GetPriceByCoingeckoId fetches the price of a token from Coingecko.
// Concurrent calls are batched into a single request, see priceBatcher.
func (c *coingeckoPricing) GetPriceByCoingeckoId(ctx context.Context, baseDenom string, coingeckoId string) (osmomath.BigDec, error) {
	if coingeckoId == "" {
		return osmomath.BigDec{}, domain.NoPriceError{Err: fmt.Errorf("coingecko ID is empty for base (%s)", baseDenom)}
	}

	result, expiresAt, err := c.batcher.getPrice(ctx, coingeckoId)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Never let this cache serve the price past the batcher's own deadline for it.
	if ttl := min(c.cacheExpiryNs, time.Until(expiresAt)); ttl > 0 {
		cacheKey := domain.FormatPricingCacheKey(baseDenom, c.quoteCurrency)
		c.cache.Set(cacheKey, result, ttl)
	}

	return result, nil
}

// refreshInterval returns the configured bulk refresh interval, or the default when unset.
func refreshInterval(config domain.PricingConfig) time.Duration {
	if config.CoingeckoRefreshIntervalMs <= 0 {
		return defaultRefreshInterval
	}
	return time.Duration(config.CoingeckoRefreshIntervalMs) * time.Millisecond
}

// priceBatcher coalesces Coingecko price lookups and keeps the prices of recently used ids fresh.
//
// A lookup that misses the cache joins the ids requested within batchWindow, which are fetched
// with one request per maxIdsPerRequest ids; callers waiting on the same id share one result.
// Every id looked up in the last trackWindow is then re-fetched in bulk every refreshInterval, so
// lookups are normally served from the cache. A price is served for at most priceTTL after the
// request that fetched it started: when a refresh fails, prices expire and the next lookup fetches
// on demand, so no older price is ever served. An id Coingecko has no price for is answered as
// unpriced for noPriceCacheTTL. Other failures (HTTP errors, zero prices) are not cached. A result
// never replaces one from a request that started later. No request is sent while a 429 backoff is
// active.
type priceBatcher struct {
	url             string
	quoteCurrency   string
	refreshInterval time.Duration
	priceTTL        time.Duration

	mu      sync.Mutex
	cache   map[string]cachedPrice
	noPrice map[string]time.Time
	// resultAt is the start time of the request whose result is stored for an id, so a slower,
	// older response cannot overwrite a newer one.
	resultAt     map[string]time.Time
	tracked      map[string]time.Time
	pending      map[string]*priceCall
	queue        []string
	flushArmed   bool
	refreshArmed bool
	backoffUntil time.Time
}

type cachedPrice struct {
	price     osmomath.BigDec
	expiresAt time.Time
}

// priceCall is the shared result for one id; done is closed once price (with expiresAt) or err is set.
type priceCall struct {
	done      chan struct{}
	price     osmomath.BigDec
	expiresAt time.Time
	err       error
}

func newPriceBatcher(url string, quoteCurrency string, refreshInterval time.Duration) *priceBatcher {
	return &priceBatcher{
		url:             url,
		quoteCurrency:   quoteCurrency,
		refreshInterval: refreshInterval,
		// The grace past one interval covers a refresh that takes a while to complete.
		priceTTL: refreshInterval + refreshInterval/5,
		cache:    make(map[string]cachedPrice),
		noPrice:  make(map[string]time.Time),
		resultAt: make(map[string]time.Time),
		tracked:  make(map[string]time.Time),
		pending:  make(map[string]*priceCall),
	}
}

// getPrice returns the price for the given Coingecko id and when it stops being servable, waiting
// for the batch it joins. The caller's ctx only bounds its own wait; it never cancels the shared request.
func (b *priceBatcher) getPrice(ctx context.Context, coingeckoId string) (osmomath.BigDec, time.Time, error) {
	b.mu.Lock()
	now := time.Now()
	b.track(coingeckoId, now)
	if cached, ok := b.cache[coingeckoId]; ok && now.Before(cached.expiresAt) {
		b.mu.Unlock()
		return cached.price, cached.expiresAt, nil
	}

	if until, ok := b.noPrice[coingeckoId]; ok && now.Before(until) {
		b.mu.Unlock()
		return osmomath.BigDec{}, time.Time{}, domain.NoPriceError{Err: fmt.Errorf("price not found for coingecko ID: %s", coingeckoId)}
	}

	if now.Before(b.backoffUntil) {
		until := b.backoffUntil
		b.mu.Unlock()
		return osmomath.BigDec{}, time.Time{}, fmt.Errorf("coingecko rate limited until %s, no price for coingecko ID: %s", until.UTC().Format(time.RFC3339), coingeckoId)
	}

	call, ok := b.pending[coingeckoId]
	if !ok {
		call = &priceCall{done: make(chan struct{})}
		b.pending[coingeckoId] = call
		b.queue = append(b.queue, coingeckoId)
		if !b.flushArmed {
			b.flushArmed = true
			time.AfterFunc(batchWindow, b.flush)
		}
	}
	b.mu.Unlock()

	select {
	case <-call.done:
		return call.price, call.expiresAt, call.err
	case <-ctx.Done():
		return osmomath.BigDec{}, time.Time{}, ctx.Err()
	}
}

// track records a lookup of id so the bulk refresh keeps its price fresh, and arms the refresh
// if it is not running. Must be called with b.mu held.
func (b *priceBatcher) track(id string, now time.Time) {
	b.tracked[id] = now
	if !b.refreshArmed {
		b.refreshArmed = true
		time.AfterFunc(b.refreshInterval, b.refresh)
	}
}

// refresh re-fetches every id looked up within trackWindow, in chunks of maxIdsPerRequest, and
// re-arms itself while any id is tracked. Ids not looked up within trackWindow are dropped.
func (b *priceBatcher) refresh() {
	b.mu.Lock()
	now := time.Now()
	ids := make([]string, 0, len(b.tracked))
	for id, lastLookup := range b.tracked {
		if now.Sub(lastLookup) > trackWindow {
			delete(b.tracked, id)
			delete(b.cache, id)
			delete(b.noPrice, id)
			delete(b.resultAt, id)
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		b.refreshArmed = false
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	// A stable order keeps request URLs identical between refreshes, so caches in front of
	// Coingecko can serve repeats.
	sort.Strings(ids)
	for start := 0; start < len(ids); start += maxIdsPerRequest {
		end := min(start+maxIdsPerRequest, len(ids))
		b.fetchChunk(ids[start:end], domain.CoingeckoTriggerRefresh)
	}

	time.AfterFunc(b.refreshInterval, b.refresh)
}

// flush sends the queued ids in chunks of maxIdsPerRequest, one request after another so that
// a 429 stops the remaining chunks from being sent.
func (b *priceBatcher) flush() {
	b.mu.Lock()
	ids := b.queue
	b.queue = nil
	b.flushArmed = false
	b.mu.Unlock()

	for start := 0; start < len(ids); start += maxIdsPerRequest {
		end := min(start+maxIdsPerRequest, len(ids))
		b.fetchChunk(ids[start:end], domain.CoingeckoTriggerOnDemand)
	}
}

// fetchChunk requests prices for ids, stores the results and resolves any pending calls for them.
func (b *priceBatcher) fetchChunk(ids []string, trigger string) {
	b.mu.Lock()
	backoffUntil := b.backoffUntil
	b.mu.Unlock()

	var (
		prices map[string]float64
		err    error
	)
	// Freshness is measured from when the request started, the earliest the price could be from.
	started := time.Now()
	if started.Before(backoffUntil) {
		err = fmt.Errorf("coingecko rate limited until %s", backoffUntil.UTC().Format(time.RFC3339))
	} else {
		prices, err = b.request(ids, trigger)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	expiresAt := started.Add(b.priceTTL)
	for _, id := range ids {
		var (
			price    osmomath.BigDec
			priceErr error
		)
		if err != nil {
			// A failed request leaves cached prices to expire on their own; it never extends them.
			priceErr = fmt.Errorf("no price for coingecko ID %s: %w", id, err)
		} else {
			price, priceErr = parsePrice(id, prices)
			if last, ok := b.resultAt[id]; !ok || !last.After(started) {
				b.resultAt[id] = started
				switch {
				case priceErr == nil:
					b.cache[id] = cachedPrice{price: price, expiresAt: expiresAt}
					delete(b.noPrice, id)
				case domain.IsNoPrice(priceErr):
					delete(b.cache, id)
					b.noPrice[id] = started.Add(noPriceCacheTTL)
				default:
					delete(b.cache, id)
				}
			}
		}

		if call, ok := b.pending[id]; ok && trigger == domain.CoingeckoTriggerOnDemand {
			delete(b.pending, id)
			call.price, call.expiresAt, call.err = price, expiresAt, priceErr
			close(call.done)
		}
	}
}

// request fetches prices for ids in one Coingecko request. It is not tied to any caller's
// context so that one cancelled caller cannot fail the batch for the others; the shared
// client's timeout bounds it.
func (b *priceBatcher) request(ids []string, trigger string) (map[string]float64, error) {
	domain.SQSPricingCoingeckoRequestsCounter.WithLabelValues(trigger).Inc()
	domain.SQSPricingCoingeckoRequestedIdsCounter.WithLabelValues(trigger).Add(float64(len(ids)))

	url := fmt.Sprintf("%s?ids=%s&vs_currencies=%s", b.url, strings.Join(ids, ","), b.quoteCurrency)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := sqshttp.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		backoff := parseRetryAfter(resp.Header.Get("Retry-After"))
		b.mu.Lock()
		if until := time.Now().Add(backoff); until.After(b.backoffUntil) {
			b.backoffUntil = until
		}
		b.mu.Unlock()
		return nil, fmt.Errorf("failed to get price from Coingecko: %s, backing off for %s", resp.Status, backoff)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get price from Coingecko: %s", resp.Status)
	}

	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode Coingecko response: %s", err)
	}

	prices := make(map[string]float64, len(data))
	for id, quotes := range data {
		if price, ok := quotes[b.quoteCurrency]; ok {
			prices[id] = price
		}
	}

	return prices, nil
}

// parsePrice converts the Coingecko price for id, rejecting missing and non-positive prices.
func parsePrice(id string, prices map[string]float64) (osmomath.BigDec, error) {
	price, ok := prices[id]
	if !ok {
		return osmomath.BigDec{}, domain.NoPriceError{Err: fmt.Errorf("price not found for coingecko ID: %s", id)}
	}

	// Format with the shortest exact representation rather than %f, which rounds to 6 decimals
	// and turns prices below 5e-7 into zero.
	result, err := osmomath.NewBigDecFromStr(strconv.FormatFloat(price, 'f', -1, 64))
	if err != nil {
		return osmomath.BigDec{}, err
	}

	if !result.IsPositive() {
		return osmomath.BigDec{}, fmt.Errorf("non-positive price from Coingecko for coingecko ID: %s", id)
	}

	return result, nil
}

// parseRetryAfter returns the delay from a Retry-After header (seconds or HTTP date), capped at maxRateLimitBackoff.
func parseRetryAfter(value string) time.Duration {
	backoff := defaultRateLimitBackoff
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		backoff = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(value); err == nil {
		backoff = time.Until(date)
	}

	return min(max(backoff, 0), maxRateLimitBackoff)
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

// isAllowedQuoteDenom returns true if quoteDenom is empty, the configured default quote denom,
// or one of the USD stablecoin denoms.
func (c *coingeckoPricing) isAllowedQuoteDenom(quoteDenom string) bool {
	if strings.TrimSpace(quoteDenom) == "" {
		return true
	}

	if c.defaultQuoteDenom != "" && quoteDenom == c.defaultQuoteDenom {
		return true
	}

	return quoteDenom == USDC_DENOM || quoteDenom == NOBLE_USDC_DENOM || quoteDenom == USDT_DENOM
}
