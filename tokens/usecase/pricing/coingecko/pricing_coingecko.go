package coingeckopricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	// maxRefreshInterval caps the configured interval, which also bounds how old a served price can be.
	maxRefreshInterval = 10 * time.Minute
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
	TUsecase mvc.TokensUsecase
	// cache is kept for domain.PricingSource; prices are cached by the batcher, which is the only
	// layer that knows when a price stops being servable.
	cache         *cache.Cache
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

	result, err := c.batcher.getPrice(ctx, coingeckoId)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return result, nil
}

// minRefreshInterval keeps the interval (and so a price's lifetime) above the time one refresh of
// Coingecko can take, so prices are not expired by the time their request completes. A variable
// only so tests can use shorter intervals.
var minRefreshInterval = 10 * time.Second

// refreshInterval returns the configured bulk refresh interval within [minRefreshInterval,
// maxRefreshInterval], or the default when unset.
func refreshInterval(config domain.PricingConfig) time.Duration {
	if config.CoingeckoRefreshIntervalMs <= 0 {
		return defaultRefreshInterval
	}
	return clampRefreshInterval(config.CoingeckoRefreshIntervalMs, minRefreshInterval, maxRefreshInterval)
}

// clampRefreshInterval converts ms to a duration within [lo, hi]. It compares against hi before
// converting, so a huge configured value cannot overflow time.Duration.
func clampRefreshInterval(ms int, lo, hi time.Duration) time.Duration {
	if int64(ms) >= int64(hi/time.Millisecond) {
		return hi
	}
	return max(time.Duration(ms)*time.Millisecond, lo)
}

// priceBatcher coalesces Coingecko price lookups and keeps the prices of recently used ids fresh.
//
// A lookup that misses the cache joins the ids requested within batchWindow, which are fetched
// with one request per maxIdsPerRequest ids; callers waiting on the same id share one result.
// Every id looked up in the last trackWindow is then re-fetched in bulk every refreshInterval, so
// lookups are normally served from the cache. A price is served for at most priceTTL after the
// request that fetched it started, and a request for an id that fails removes its price, so the
// next lookup fetches on demand and an outage leaves the id unpriced rather than served an old
// price. A rate limit is not such a failure: it says nothing about the price, which is left to
// expire on its own. An id Coingecko has no price for is answered as unpriced for
// noPriceCacheTTL. Other failures (HTTP errors, zero prices) are not cached. A result never
// replaces one from a request that started later. No request is sent while a 429 backoff is active.
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
	refreshTimer *time.Timer
	// stopped ends the refresh. Only tests set it (see stop): in production the batcher refreshes
	// for the life of the process.
	stopped      bool
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

func newPriceBatcher(coingeckoUrl string, quoteCurrency string, refreshInterval time.Duration) *priceBatcher {
	return &priceBatcher{
		url:             coingeckoUrl,
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

// getPrice returns the price for the given Coingecko id, waiting for the batch it joins.
// The caller's ctx only bounds its own wait; it never cancels the shared request.
func (b *priceBatcher) getPrice(ctx context.Context, coingeckoId string) (osmomath.BigDec, error) {
	b.mu.Lock()
	now := time.Now()
	b.track(coingeckoId, now)
	if price, ok, err := b.stored(coingeckoId, now); ok {
		b.mu.Unlock()
		domain.SQSPricingCoingeckoCacheHitsCounter.Inc()
		return price, err
	}
	domain.SQSPricingCoingeckoCacheMissesCounter.Inc()

	if now.Before(b.backoffUntil) {
		until := b.backoffUntil
		b.mu.Unlock()
		return osmomath.BigDec{}, fmt.Errorf("coingecko rate limited until %s, no price for coingecko ID: %s", until.UTC().Format(time.RFC3339), coingeckoId)
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
		// The price can expire between being handed over and this lookup resuming.
		if call.err == nil && !time.Now().Before(call.expiresAt) {
			return osmomath.BigDec{}, fmt.Errorf("coingecko price for ID %s expired before it was returned", coingeckoId)
		}
		return call.price, call.err
	case <-ctx.Done():
		return osmomath.BigDec{}, ctx.Err()
	}
}

// stored returns the servable stored answer for id, a price or a no-price error, and false when
// there is none. Must be called with b.mu held.
func (b *priceBatcher) stored(id string, now time.Time) (osmomath.BigDec, bool, error) {
	if cached, ok := b.cache[id]; ok && now.Before(cached.expiresAt) {
		return cached.price, true, nil
	}
	if until, ok := b.noPrice[id]; ok && now.Before(until) {
		return osmomath.BigDec{}, true, domain.NoPriceError{Err: fmt.Errorf("price not found for coingecko ID: %s", id)}
	}
	return osmomath.BigDec{}, false, nil
}

// track records a lookup of id so the bulk refresh keeps its price fresh, and arms the refresh
// if it is not running. Must be called with b.mu held.
func (b *priceBatcher) track(id string, now time.Time) {
	b.tracked[id] = now
	if !b.refreshArmed {
		b.armRefresh(b.refreshInterval)
	}
}

// armRefresh schedules the next refresh after delay, unless the batcher is stopped. Must be
// called with b.mu held.
func (b *priceBatcher) armRefresh(delay time.Duration) {
	b.refreshArmed = !b.stopped
	if b.stopped {
		return
	}
	b.refreshTimer = time.AfterFunc(delay, b.refresh)
}

// refresh re-fetches every id looked up within trackWindow, in chunks of maxIdsPerRequest, and
// re-arms itself while any id is tracked. Ids not looked up within trackWindow are dropped.
func (b *priceBatcher) refresh() {
	b.mu.Lock()
	if b.stopped {
		b.refreshArmed = false
		b.mu.Unlock()
		return
	}
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
		// An id Coingecko has no price for is asked about again only once its no-price answer expires.
		if until, ok := b.noPrice[id]; ok && now.Before(until) {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		// Every tracked id may be waiting out a no-price answer; keep refreshing while any is tracked.
		if len(b.tracked) == 0 {
			b.refreshArmed = false
		} else {
			b.armRefresh(b.refreshInterval)
		}
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

	// Timed from the start of this cycle, so each id is re-fetched about one interval after its
	// last fetch began, inside its priceTTL, however long the cycle took. A cycle that overran
	// still leaves a short gap before the next.
	b.mu.Lock()
	b.armRefresh(max(b.refreshInterval-time.Since(now), b.refreshInterval/10))
	b.mu.Unlock()
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
	// Freshness is measured from when the request started, the earliest the price could be from.
	started := time.Now()
	prices, failed, err := b.request(ids, trigger)

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	expiresAt := started.Add(b.priceTTL)
	for _, id := range ids {
		var (
			price    osmomath.BigDec
			priceErr error
			// informative is false for a result that says nothing about the id's price: it is not
			// stored, and waiters get the stored answer if one is still servable.
			informative = true
		)
		switch {
		case err != nil:
			// A request that failed removes the price: once the source is known to be failing, the
			// old price is not served for the rest of its lifetime. A rate limit, or a chunk not
			// sent because of one, leaves prices to expire on their own.
			priceErr = fmt.Errorf("no price for coingecko ID %s: %w", id, err)
			informative = failed
		case !now.Before(expiresAt):
			// The request took longer than a price may be served for.
			priceErr = fmt.Errorf("coingecko price for ID %s expired before its request completed", id)
			informative = false
		default:
			price, priceErr = parsePrice(id, prices)
			// An id missing from the answer while its price is still servable is more likely a
			// dropped answer than a price Coingecko stopped having: the price expires on its own
			// and the next refresh asks again, instead of the id being marked unpriced.
			if domain.IsNoPrice(priceErr) {
				if _, ok, _ := b.stored(id, now); ok {
					informative = false
				}
			}
		}

		if last, ok := b.resultAt[id]; informative && (!ok || !last.After(started)) {
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

		call, ok := b.pending[id]
		if !ok || trigger != domain.CoingeckoTriggerOnDemand {
			continue
		}
		delete(b.pending, id)
		call.price, call.expiresAt, call.err = price, expiresAt, priceErr
		// When this result was not stored, or a newer request's result is: hand waiters only the
		// stored answer.
		if last := b.resultAt[id]; !informative || last.After(started) {
			var ok bool
			if call.price, ok, call.err = b.stored(id, now); !ok {
				call.price, call.err = osmomath.BigDec{}, priceErr
				if call.err == nil {
					call.err = fmt.Errorf("no current price for coingecko ID %s: a newer request returned none", id)
				}
			} else if call.err == nil {
				call.expiresAt = b.cache[id].expiresAt
			}
		}
		close(call.done)
	}
}

// request fetches prices for ids in one Coingecko request. On error it reports whether the request
// failed, as opposed to not being answered because of a rate limit (a 429, or a backoff that kept
// it from being sent), which says nothing about the prices. It is not tied to any caller's context
// so that one cancelled caller cannot fail the batch for the others; the shared client's timeout
// bounds it.
func (b *priceBatcher) request(ids []string, trigger string) (map[string]float64, bool, error) {
	// Checked right before sending, so a 429 from an earlier chunk stops this one.
	b.mu.Lock()
	backoffUntil := b.backoffUntil
	b.mu.Unlock()
	if time.Now().Before(backoffUntil) {
		return nil, false, fmt.Errorf("coingecko rate limited until %s", backoffUntil.UTC().Format(time.RFC3339))
	}

	// Escaped, so an id with a reserved character cannot cut off the ids after it in a bulk request.
	query := url.Values{"ids": {strings.Join(ids, ",")}, "vs_currencies": {b.quoteCurrency}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, b.url+"?"+query.Encode(), nil)
	if err != nil {
		return nil, false, err
	}

	domain.SQSPricingCoingeckoRequestsCounter.WithLabelValues(trigger).Inc()
	domain.SQSPricingCoingeckoRequestedIdsCounter.WithLabelValues(trigger).Add(float64(len(ids)))
	resp, err := sqshttp.DefaultClient.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		backoff := parseRetryAfter(resp.Header.Get("Retry-After"))
		b.mu.Lock()
		if until := time.Now().Add(backoff); until.After(b.backoffUntil) {
			b.backoffUntil = until
		}
		b.mu.Unlock()
		return nil, false, fmt.Errorf("failed to get price from Coingecko: %s, backing off for %s", resp.Status, backoff)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, true, fmt.Errorf("failed to get price from Coingecko: %s", resp.Status)
	}

	var data map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, true, fmt.Errorf("failed to decode Coingecko response: %s", err)
	}
	// Coingecko leaves out only ids it does not know, so an empty answer to several ids is more
	// likely a dropped response than none of them being known: it is not taken as a no-price answer.
	if len(data) == 0 && len(ids) > 1 {
		return nil, false, fmt.Errorf("empty Coingecko response for %d ids", len(ids))
	}

	prices := make(map[string]float64, len(data))
	for id, quotes := range data {
		if price, ok := quotes[b.quoteCurrency]; ok {
			prices[id] = price
		}
	}

	return prices, true, nil
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
