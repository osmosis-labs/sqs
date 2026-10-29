package coingeckopricing

import (
	"context"
	"encoding/json"
	"errors"
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
	// missingPriceRecheck is how long an id that had a price is answered as unpriced after it is
	// missing from an answer: long enough that an id Coingecko dropped is not re-requested on every
	// lookup, short enough that a dropped answer is soon asked about again.
	missingPriceRecheck = 10 * time.Second
	// defaultRateLimitBackoff is used on HTTP 429 when Retry-After is missing, unparsable or not in the future.
	defaultRateLimitBackoff = 10 * time.Second
	// maxRateLimitBackoff caps the Retry-After delay honoured on HTTP 429.
	maxRateLimitBackoff = 30 * time.Second
	// minRateLimitRetry is the shortest wait before a refresh retries a chunk held back by a rate limit.
	minRateLimitRetry = time.Second
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
	cache *cache.Cache
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
		TUsecase: tokenUseCase,
		cache:    cache.New(),
		batcher:  newPriceBatcher(config.CoingeckoUrl, config.CoingeckoQuoteCurrency, refreshInterval(config.CoingeckoRefreshIntervalMs)),
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

	return c.batcher.getPrice(ctx, coingeckoId)
}

// minRefreshInterval keeps the interval (and so a price's lifetime) above the time one refresh of
// Coingecko can take, so prices are not expired by the time their request completes. A variable
// only so tests can use shorter intervals.
var minRefreshInterval = 10 * time.Second

// refreshInterval returns the configured bulk refresh interval within [minRefreshInterval,
// maxRefreshInterval], or the default when unset. It compares against the maximum before
// converting, so a huge configured value cannot overflow time.Duration.
func refreshInterval(ms int) time.Duration {
	switch {
	case ms <= 0:
		return defaultRefreshInterval
	case int64(ms) >= int64(maxRefreshInterval/time.Millisecond):
		return maxRefreshInterval
	}
	return max(time.Duration(ms)*time.Millisecond, minRefreshInterval)
}

// priceBatcher coalesces Coingecko price lookups and keeps the prices of recently used ids fresh.
//
// A lookup that misses the cache joins the ids requested within batchWindow, which are fetched
// with one request per maxIdsPerRequest ids. Every id looked up in the last trackWindow is then
// re-fetched in bulk every refreshInterval, so lookups are normally served from the cache.
//
// A price is served for at most priceTTL after the request that fetched it started. A request
// that fails removes the price, so an outage leaves the id unpriced rather than served an old
// price. A rate limit says nothing about the price, which is left to expire on its own; no request
// is sent while its backoff is active. An id Coingecko has no price for is answered as unpriced
// for noPriceCacheTTL, or briefly if it had a price, since that is more likely a dropped answer.
type priceBatcher struct {
	url             string
	quoteCurrency   string
	refreshInterval time.Duration
	priceTTL        time.Duration
	// quit is closed to end the bulk refresh. Only tests close it (see stop): in production the
	// batcher refreshes for the life of the process.
	quit chan struct{}

	mu             sync.Mutex
	cache          map[string]cachedPrice
	noPrice        map[string]time.Time
	tracked        map[string]time.Time
	pending        map[string]*priceCall
	queue          []string
	flushArmed     bool
	refreshRunning bool
	backoffUntil   time.Time
}

type cachedPrice struct {
	price     osmomath.BigDec
	expiresAt time.Time
}

// priceCall is shared by the lookups waiting on one id. done is closed once the request for it has
// stored its result; err is the error that request gave for the id.
type priceCall struct {
	done chan struct{}
	err  error
}

// errRateLimited marks a request not answered because of a rate limit: a 429, or a backoff that
// kept it from being sent.
var errRateLimited = errors.New("coingecko rate limited")

func newPriceBatcher(coingeckoUrl string, quoteCurrency string, refreshInterval time.Duration) *priceBatcher {
	return &priceBatcher{
		url:             coingeckoUrl,
		quoteCurrency:   quoteCurrency,
		refreshInterval: refreshInterval,
		// The grace past one interval covers a refresh that takes a while to complete.
		priceTTL: refreshInterval + refreshInterval/5,
		quit:     make(chan struct{}),
		cache:    make(map[string]cachedPrice),
		noPrice:  make(map[string]time.Time),
		tracked:  make(map[string]time.Time),
		pending:  make(map[string]*priceCall),
	}
}

// getPrice returns the price for the given Coingecko id, waiting for the batch it joins.
// The caller's ctx only bounds its own wait; it never cancels the shared request.
func (b *priceBatcher) getPrice(ctx context.Context, id string) (osmomath.BigDec, error) {
	b.mu.Lock()
	now := time.Now()
	b.tracked[id] = now
	if !b.refreshRunning {
		b.startRefresh(b.refreshInterval)
	}
	if price, ok, err := b.stored(id, now); ok {
		b.mu.Unlock()
		domain.SQSPricingCoingeckoCacheHitsCounter.Inc()
		return price, err
	}
	domain.SQSPricingCoingeckoCacheMissesCounter.Inc()

	if now.Before(b.backoffUntil) {
		until := b.backoffUntil
		b.mu.Unlock()
		return osmomath.BigDec{}, fmt.Errorf("%w until %s, no price for coingecko ID: %s", errRateLimited, until.UTC().Format(time.RFC3339), id)
	}

	call, ok := b.pending[id]
	if !ok {
		call = &priceCall{done: make(chan struct{})}
		b.pending[id] = call
		b.queue = append(b.queue, id)
		if !b.flushArmed {
			b.flushArmed = true
			time.AfterFunc(batchWindow, b.flush)
		}
	}
	b.mu.Unlock()

	select {
	case <-call.done:
		return b.answer(id, call.err)
	case <-ctx.Done():
		return osmomath.BigDec{}, ctx.Err()
	}
}

// answer returns the stored answer for id after its request completed, or requestErr, the error
// that request gave for it, when none is servable.
func (b *priceBatcher) answer(id string, requestErr error) (osmomath.BigDec, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if price, ok, err := b.stored(id, time.Now()); ok {
		return price, err
	}
	if requestErr != nil {
		return osmomath.BigDec{}, requestErr
	}
	return osmomath.BigDec{}, fmt.Errorf("no current price for coingecko ID %s", id)
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

// startRefresh starts the bulk refresh, first after delay. Must be called with b.mu held.
func (b *priceBatcher) startRefresh(delay time.Duration) {
	b.refreshRunning = true
	go b.refreshLoop(delay)
}

// refreshLoop re-fetches every id looked up within trackWindow, timing each cycle from the start
// of the previous one so each id is re-fetched inside its priceTTL however long a cycle took. It
// ends once no id is tracked, or when the batcher is stopped.
func (b *priceBatcher) refreshLoop(delay time.Duration) {
	for b.sleep(delay) {
		started := time.Now()
		ids, ok := b.dueIds(started)
		if !ok {
			return
		}
		// A stable order keeps request URLs identical between refreshes, so caches in front of
		// Coingecko can serve repeats.
		sort.Strings(ids)
		b.fetchRefresh(ids, started.Add(b.refreshInterval))
		delay = nextRefreshDelay(b.refreshInterval, time.Since(started))
	}
}

// nextRefreshDelay is the wait before the next cycle after one that took elapsed: one interval
// after it started, and a tenth of an interval after it ended if it overran.
func nextRefreshDelay(interval, elapsed time.Duration) time.Duration {
	return max(interval-elapsed, interval/10)
}

// dueIds returns the ids a refresh fetches: those looked up within trackWindow that are not
// waiting out a no-price answer. Ids not looked up within trackWindow are dropped; once none is
// left it reports false and the refresh ends.
func (b *priceBatcher) dueIds(now time.Time) ([]string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ids := make([]string, 0, len(b.tracked))
	for id, lastLookup := range b.tracked {
		if now.Sub(lastLookup) > trackWindow {
			delete(b.tracked, id)
			delete(b.cache, id)
			delete(b.noPrice, id)
			continue
		}
		if until, ok := b.noPrice[id]; ok && now.Before(until) {
			continue
		}
		ids = append(ids, id)
	}
	b.refreshRunning = len(b.tracked) > 0
	return ids, b.refreshRunning
}

// fetchRefresh fetches ids in chunks of maxIdsPerRequest. A chunk held back by a rate limit is
// retried once the backoff ends, so later chunks are still refreshed and earlier ones are not
// sent again. It gives up at deadline, when the next cycle starts over, or once the batcher is
// stopped. Every retry waits at least minRateLimitRetry, which bounds the loop.
func (b *priceBatcher) fetchRefresh(ids []string, deadline time.Time) {
	for start := 0; start < len(ids); {
		end := min(start+maxIdsPerRequest, len(ids))
		if !b.fetchChunk(ids[start:end], domain.CoingeckoTriggerRefresh) {
			start = end
			continue
		}

		b.mu.Lock()
		wait := max(time.Until(b.backoffUntil), minRateLimitRetry)
		b.mu.Unlock()
		if time.Now().Add(wait).After(deadline) || !b.sleep(wait) {
			return
		}
	}
}

// sleep waits for d and reports false if the batcher was stopped meanwhile.
func (b *priceBatcher) sleep(d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-b.quit:
		return false
	}
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

// fetchChunk requests prices for ids, stores the results and releases the lookups waiting on them.
// It reports whether the chunk went unanswered because of a rate limit.
func (b *priceBatcher) fetchChunk(ids []string, trigger string) (rateLimited bool) {
	// Freshness is measured from when the request started, the earliest the price could be from.
	started := time.Now()
	prices, err := b.request(ids, trigger)

	b.mu.Lock()
	defer b.mu.Unlock()
	for _, id := range ids {
		var (
			price osmomath.BigDec
			idErr error
		)
		if err == nil {
			price, idErr = parsePrice(id, prices)
		} else {
			idErr = fmt.Errorf("no price for coingecko ID %s: %w", id, err)
		}
		b.store(id, price, idErr, started)

		// Lookups wait only on on-demand requests, which every queued id gets.
		if call, ok := b.pending[id]; ok && trigger == domain.CoingeckoTriggerOnDemand {
			delete(b.pending, id)
			call.err = idErr
			close(call.done)
		}
	}
	return errors.Is(err, errRateLimited)
}

// store records what a request started at started said about id's price. Must be called with b.mu held.
func (b *priceBatcher) store(id string, price osmomath.BigDec, err error, started time.Time) {
	expiresAt := started.Add(b.priceTTL)
	cached, hasPrice := b.cache[id]
	// A slower response to an older request never replaces a price from a newer one.
	if hasPrice && cached.expiresAt.After(expiresAt) {
		return
	}

	switch {
	case err == nil:
		b.cache[id] = cachedPrice{price: price, expiresAt: expiresAt}
		delete(b.noPrice, id)
	case errors.Is(err, errRateLimited):
		// Says nothing about the price, which is left to expire on its own.
	case domain.IsNoPrice(err):
		// While the price is servable, an id missing from an answer is more likely a dropped answer
		// than a price Coingecko stopped having: the price is kept. An id that had a price is then
		// asked about again soon, and one that had none is not asked about for noPriceCacheTTL.
		if hasPrice && time.Now().Before(cached.expiresAt) {
			return
		}
		ttl := noPriceCacheTTL
		if hasPrice {
			ttl = missingPriceRecheck
		}
		delete(b.cache, id)
		b.noPrice[id] = started.Add(ttl)
	default:
		// Once the source is known to be failing, the old price is not served for the rest of its lifetime.
		delete(b.cache, id)
	}
}

// request fetches prices for ids in one Coingecko request. An error wrapping errRateLimited means
// the request was not answered because of a rate limit; any other error means it failed. It is
// not tied to any caller's context so that one cancelled caller cannot fail the batch for the
// others; the shared client's timeout bounds it.
func (b *priceBatcher) request(ids []string, trigger string) (map[string]float64, error) {
	// Checked right before sending, so a 429 from an earlier chunk stops this one.
	b.mu.Lock()
	backoffUntil := b.backoffUntil
	b.mu.Unlock()
	if time.Now().Before(backoffUntil) {
		return nil, fmt.Errorf("%w until %s", errRateLimited, backoffUntil.UTC().Format(time.RFC3339))
	}

	// Escaped, so an id with a reserved character cannot cut off the ids after it in a bulk request.
	query := url.Values{"ids": {strings.Join(ids, ",")}, "vs_currencies": {b.quoteCurrency}}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, b.url+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}

	domain.SQSPricingCoingeckoRequestsCounter.WithLabelValues(trigger).Inc()
	domain.SQSPricingCoingeckoRequestedIdsCounter.WithLabelValues(trigger).Add(float64(len(ids)))
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
		return nil, fmt.Errorf("%w: %s, backing off for %s", errRateLimited, resp.Status, backoff)
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

// parseRetryAfter returns the delay from a Retry-After header (seconds or HTTP date), capped at
// maxRateLimitBackoff. A delay that is missing, unparsable or not in the future falls back to
// defaultRateLimitBackoff: the 429 itself says to slow down, so it never means retry at once.
func parseRetryAfter(value string) time.Duration {
	backoff := time.Duration(0)
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil {
		backoff = time.Duration(seconds) * time.Second
	} else if date, err := http.ParseTime(value); err == nil {
		backoff = time.Until(date)
	}
	if backoff <= 0 {
		return defaultRateLimitBackoff
	}

	return min(backoff, maxRateLimitBackoff)
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
