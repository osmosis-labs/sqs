// Package coingeckopoller prices tokens from Coingecko out of an in-memory snapshot.
//
// The Coingecko ids come only from the asset list, which is small (about 170 unique ids), so one
// background loop fetches all of them every interval and publishes the result as an immutable
// snapshot. Lookups read the snapshot and never wait on the network, and the request rate to
// Coingecko is fixed by the interval rather than by traffic or failures.
//
// Coingecko is a fallback, so the loop runs only on demand: it pauses after an hour without a
// lookup and resumes on the next one.
package coingeckopoller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/osmosis-labs/osmosis/osmomath"
	sqshttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
)

const (
	// USDC_DENOM is the alloyed USDC (allUSDC), the default quote denom for SQS prices.
	USDC_DENOM = "factory/osmo147h5x9pcj7lm0cttlaefx6sqq5vdfnmwfcqxkmjd7exqm9gc7grqhr75m0/alloyed/allUSDC"
	// NOBLE_USDC_DENOM is the Noble USDC variant, the previous default quote denom.
	NOBLE_USDC_DENOM = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	USDT_DENOM       = "ibc/4ABBEF4C8926DDDB320AE5188CFD63267ABBCEFC0583E4AE05D6E5AA2401DDAB"
)

const (
	// maxIdsPerRequest caps the number of Coingecko ids in a single request.
	maxIdsPerRequest = 100
	// defaultInterval is how often every id is re-fetched when the config leaves it unset.
	defaultInterval = 5 * time.Minute
	// minInterval and maxInterval bound the configured interval.
	minInterval = 10 * time.Second
	maxInterval = 10 * time.Minute
	// minMaxAgeGrace is the least time an answer is used for past one interval (see maxAge).
	minMaxAgeGrace = 30 * time.Second
	// minBackoff is the first wait before retrying a failed refresh; it doubles up to the interval.
	minBackoff = time.Second
	// minRateLimitBackoff is the shortest wait after HTTP 429, used when Retry-After is missing:
	// the 429 itself says to slow down.
	minRateLimitBackoff = 10 * time.Second
	// demandWindow is how long refreshes go on after the last lookup, as the batcher this replaces
	// kept refreshing an id for an hour after it was last looked up.
	demandWindow = time.Hour
)

var (
	// errUnavailable marks a lookup that cannot be answered because Coingecko has not answered for
	// the id recently, as opposed to Coingecko answering that it has no price for it.
	errUnavailable = errors.New("coingecko prices unavailable")
	// errNoIds is returned by a refresh while the token metadata has no Coingecko ids, before it is loaded.
	errNoIds = errors.New("no coingecko ids: token metadata not loaded")
)

// entry is what Coingecko last said about one id.
type entry struct {
	price osmomath.BigDec
	// pricedAt is when the request that returned price started, the earliest the price could be
	// from; zero if Coingecko has not returned a price for the id.
	pricedAt time.Time
	// answeredAt is when the last request answered for the id started, with or without a price.
	answeredAt time.Time
}

// snapshot is never modified once published.
type snapshot struct {
	entries map[string]entry
}

// Source is a domain.PricingSource that serves Coingecko prices from a snapshot refreshed by Run.
type Source struct {
	tokens mvc.TokensUsecase
	logger log.Logger
	fetch  fetchFunc
	// defaultQuoteDenom is the chain denom resolved from config.DefaultQuoteHumanDenom. Chain
	// pricing falls back to Coingecko only for this denom, so it is always accepted as a quote denom.
	defaultQuoteDenom string
	interval          time.Duration
	maxAge            time.Duration
	now               func() time.Time
	sleep             func(ctx context.Context, d time.Duration) bool

	snap    atomic.Pointer[snapshot]
	running atomic.Bool

	// lastDemand is the unix time in nanoseconds of the latest lookup, or of the start of Run.
	lastDemand atomic.Int64
	// idle is set while Run waits for a lookup; wake carries the lookup that ends the wait.
	idle atomic.Bool
	wake chan struct{}
}

var _ domain.PricingSource = (*Source)(nil)

// New creates a Coingecko pricing source. It serves no price until Run has fetched one.
// It returns an error if the Coingecko URL or quote currency in config is invalid.
func New(tokens mvc.TokensUsecase, config domain.PricingConfig, logger log.Logger) (*Source, error) {
	fetch, err := newHTTPFetch(config.CoingeckoUrl, config.CoingeckoQuoteCurrency, sqshttp.DefaultClient)
	if err != nil {
		return nil, err
	}
	s := newSource(tokens, fetch, refreshInterval(config.CoingeckoRefreshIntervalMs), logger)

	// If the default quote human denom does not resolve, only the stablecoin denoms are accepted.
	// The chain pricing source fails at startup on the same lookup, so this is not reached in practice.
	if defaultQuoteDenom, err := tokens.GetChainDenom(config.DefaultQuoteHumanDenom); err == nil {
		s.defaultQuoteDenom = defaultQuoteDenom
	}
	return s, nil
}

func newSource(tokens mvc.TokensUsecase, fetch fetchFunc, interval time.Duration, logger log.Logger) *Source {
	if interval <= 0 {
		panic(fmt.Sprintf("coingeckopoller: non-positive refresh interval %s", interval))
	}
	s := &Source{
		tokens:   tokens,
		logger:   logger,
		fetch:    fetch,
		interval: interval,
		maxAge:   maxAge(interval),
		now:      time.Now,
		sleep:    sleep,
		wake:     make(chan struct{}, 1),
	}
	s.snap.Store(&snapshot{entries: map[string]entry{}})
	return s
}

// refreshInterval returns the configured interval within [minInterval, maxInterval], or the
// default when unset. It compares in milliseconds so a huge value cannot overflow time.Duration.
func refreshInterval(ms int) time.Duration {
	switch {
	case ms <= 0:
		return defaultInterval
	case int64(ms) >= maxInterval.Milliseconds():
		return maxInterval
	}
	return max(time.Duration(ms)*time.Millisecond, minInterval)
}

// maxAge returns how long an answer is used for after its request started: one interval plus a
// fifth of it, as the batcher this replaces served its prices (6 minutes at the default 5 minute
// interval). Prices are shown as current, so they are not served much past the next refresh.
// The grace is at least minMaxAgeGrace, so that at short intervals a slow refresh, or a failed
// one retried after a few seconds of backoff, lands before the answers it replaces expire.
func maxAge(interval time.Duration) time.Duration {
	return interval + max(interval/5, minMaxAgeGrace)
}

// GetPrice implements domain.PricingSource. Coingecko prices are in USD, which is served for a
// nil quote denom, the configured default quote denom, or a USD stablecoin: allUSDC, Noble USDC or USDT.
// It never waits on the network: a lookup that ends a pause in refreshes is answered from the
// snapshot as it is, usually expired and so unavailable, and the refresh it starts serves the next ones.
func (s *Source) GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	if !s.isAllowedQuoteDenom(quoteDenom) {
		return osmomath.BigDec{}, nil, fmt.Errorf("only the default quote denom, usdc/usdt denom or nil is allowed for the quote denom param")
	}
	id, err := s.tokens.GetCoingeckoIdByChainDenom(baseDenom)
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}
	// Trimmed as the refresh trims it, so that both use the same key.
	id = strings.TrimSpace(id)
	if id == "" {
		return osmomath.BigDec{}, nil, domain.NoPriceError{Err: fmt.Errorf("coingecko ID is empty for base (%s)", baseDenom)}
	}
	s.recordDemand()

	price, err := s.price(id)
	if err != nil {
		return osmomath.BigDec{}, nil, err
	}
	return price, nil, nil
}

// price returns a copy of the price for id if it was fetched within maxAge. Otherwise the token
// has no price if Coingecko answered for it within maxAge, and Coingecko is unavailable if not.
func (s *Source) price(id string) (osmomath.BigDec, error) {
	e, ok := s.snap.Load().entries[id]
	now := s.now()
	switch {
	case ok && !e.pricedAt.IsZero() && now.Sub(e.pricedAt) <= s.maxAge:
		domain.SQSPricingCoingeckoCacheHitsCounter.Inc()
		// A copy, so that a caller using a *Mut method cannot change the shared snapshot.
		return e.price.Clone(), nil
	case ok && now.Sub(e.answeredAt) <= s.maxAge:
		domain.SQSPricingCoingeckoCacheHitsCounter.Inc()
		return osmomath.BigDec{}, domain.NoPriceError{Err: fmt.Errorf("price not found for coingecko ID: %s", id)}
	default:
		domain.SQSPricingCoingeckoCacheMissesCounter.Inc()
		return osmomath.BigDec{}, fmt.Errorf("%w: no answer since %s, no price for coingecko ID %s",
			errUnavailable, formatTime(e.answeredAt), id)
	}
}

// Run refreshes the snapshot until ctx is done: once immediately, then one interval after the
// start of the previous refresh, and after a failed refresh sooner, with a backoff that doubles
// from minBackoff up to the interval and waits out a rate limit. Starting counts as a lookup, so
// the snapshot is filled before the first one; after demandWindow without a lookup, Run waits for
// the next one. It must be called once, since the snapshot has a single writer.
func (s *Source) Run(ctx context.Context) {
	if !s.running.CompareAndSwap(false, true) {
		panic("coingeckopoller: Run called more than once")
	}
	s.lastDemand.Store(s.now().UnixNano())

	var backoff time.Duration
	for {
		started := s.now()
		err := s.refresh(ctx)
		if ctx.Err() != nil {
			return
		}
		backoff = nextBackoff(backoff, err, s.interval)
		delay := nextRefreshDelay(s.interval, s.now().Sub(started))
		if err != nil {
			delay = backoff
			s.logger.Error("coingecko refresh failed", zap.Error(err), zap.Duration("retry_in", delay))
		}
		if !s.sleep(ctx, delay) || !s.awaitDemand(ctx) {
			return
		}
	}
}

// recordDemand notes a lookup and wakes Run if it is waiting for one.
func (s *Source) recordDemand() {
	s.lastDemand.Store(s.now().UnixNano())
	// Run sets idle before its last look at lastDemand, so either it sees this lookup or this
	// sees idle set. A wake left in the channel after Run resumed only makes its next wait look
	// at lastDemand once more.
	if s.idle.Load() {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
}

// demanded reports whether there was a lookup within demandWindow.
func (s *Source) demanded() bool {
	return s.now().Sub(time.Unix(0, s.lastDemand.Load())) <= demandWindow
}

// awaitDemand returns true at once if there was a lookup within demandWindow, and otherwise
// waits for the next lookup. It returns false if ctx is done first.
func (s *Source) awaitDemand(ctx context.Context) bool {
	if s.demanded() {
		return true
	}
	s.idle.Store(true)
	domain.SQSPricingCoingeckoIdleGauge.Set(1)
	defer func() {
		s.idle.Store(false)
		domain.SQSPricingCoingeckoIdleGauge.Set(0)
	}()
	s.logger.Info("coingecko refresh paused: no lookup in the demand window", zap.Duration("demand_window", demandWindow))

	for !s.demanded() {
		select {
		case <-s.wake:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// refresh fetches every id in chunks, the ids answered longest ago first, and publishes each chunk
// as it arrives. A failed chunk does not stop the others, except a rate limit, which stops the
// refresh; the ids it held back are then the first ones fetched by the next refresh. It returns
// the errors of the failed chunks.
func (s *Source) refresh(ctx context.Context) error {
	ids, err := s.coingeckoIds()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return errNoIds
	}
	orderByAnswer(ids, s.snap.Load())

	var errs []error
	for start := 0; start < len(ids); start += maxIdsPerRequest {
		chunk := ids[start:min(start+maxIdsPerRequest, len(ids))]
		chunkStarted := s.now()
		prices, err := s.fetch(ctx, chunk)
		if err != nil {
			errs = append(errs, err)
			var rateLimited *rateLimitError
			if errors.As(err, &rateLimited) || ctx.Err() != nil {
				break
			}
			continue
		}
		s.publish(ids, chunk, prices, chunkStarted)
	}
	return errors.Join(errs...)
}

// publish stores the answer to a request for chunk that started at answeredAt. The snapshot keeps
// only ids, the current ids from the token metadata.
func (s *Source) publish(ids, chunk []string, prices map[string]osmomath.BigDec, answeredAt time.Time) {
	prev := s.snap.Load()
	next := make(map[string]entry, len(ids))
	for _, id := range ids {
		if e, ok := prev.entries[id]; ok {
			next[id] = e
		}
	}
	for _, id := range chunk {
		e := next[id]
		e.answeredAt = answeredAt
		// An id answered without a price keeps its previous price, which expires on its own.
		if price, ok := prices[id]; ok {
			e.price, e.pricedAt = price, answeredAt
		}
		next[id] = e
	}
	s.snap.Store(&snapshot{entries: next})
	domain.SQSPricingCoingeckoOldestAnswerGauge.Set(oldestAnswer(ids, next))
}

// oldestAnswer returns the unix time of the oldest answer among ids, or 0 if any id has none.
func oldestAnswer(ids []string, entries map[string]entry) float64 {
	var oldest time.Time
	for _, id := range ids {
		e, ok := entries[id]
		if !ok {
			return 0
		}
		if oldest.IsZero() || e.answeredAt.Before(oldest) {
			oldest = e.answeredAt
		}
	}
	return float64(oldest.Unix())
}

// coingeckoIds returns the unique, trimmed Coingecko ids in the token metadata.
func (s *Source) coingeckoIds() ([]string, error) {
	tokens, err := s.tokens.GetFullTokenMetadata()
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(tokens))
	ids := make([]string, 0, len(tokens))
	for _, token := range tokens {
		id := strings.TrimSpace(token.CoingeckoID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

// orderByAnswer sorts ids by their last answer, oldest (or none) first, then by id. Ids held back
// by a rate limit are so fetched first next time instead of being starved by the ones before them,
// and in steady state every chunk holds the same ids, so request URLs stay identical between
// refreshes and caches in front of Coingecko can serve repeats.
func orderByAnswer(ids []string, snap *snapshot) {
	sort.Slice(ids, func(i, j int) bool {
		ai, aj := snap.entries[ids[i]].answeredAt, snap.entries[ids[j]].answeredAt
		if !ai.Equal(aj) {
			return ai.Before(aj)
		}
		return ids[i] < ids[j]
	})
}

// nextRefreshDelay is the wait before the next refresh after one that took elapsed: one interval
// after it started, and a tenth of an interval after it ended if it overran.
func nextRefreshDelay(interval, elapsed time.Duration) time.Duration {
	return max(interval-elapsed, interval/10)
}

// nextBackoff returns the wait before retrying after a refresh that returned err, given the
// previous backoff: zero after a success, minBackoff while the token metadata is not loaded,
// otherwise double the previous one from minBackoff up to interval, and at least what a rate
// limit asks for, which can exceed the interval.
func nextBackoff(prev time.Duration, err error, interval time.Duration) time.Duration {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errNoIds):
		return minBackoff
	}
	next := min(max(2*prev, minBackoff), interval)
	var rateLimited *rateLimitError
	if errors.As(err, &rateLimited) {
		next = max(next, rateLimited.retryAfter, minRateLimitBackoff)
	}
	return next
}

// sleep waits for d and reports false if ctx was done first.
func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "startup"
	}
	return t.UTC().Format(time.RFC3339)
}

// InitializeCache implements domain.PricingSource. Prices are kept in the snapshot, so the cache is unused.
func (s *Source) InitializeCache(*cache.Cache) {}

// GetFallbackStrategy implements domain.PricingSource. Coingecko has no fallback.
func (s *Source) GetFallbackStrategy(quoteDenom string) domain.PricingSourceType {
	return domain.NoneSourceType
}

// isAllowedQuoteDenom returns true if quoteDenom is empty, the configured default quote denom,
// or one of the USD stablecoin denoms.
func (s *Source) isAllowedQuoteDenom(quoteDenom string) bool {
	if strings.TrimSpace(quoteDenom) == "" {
		return true
	}
	if s.defaultQuoteDenom != "" && quoteDenom == s.defaultQuoteDenom {
		return true
	}
	return quoteDenom == USDC_DENOM || quoteDenom == NOBLE_USDC_DENOM || quoteDenom == USDT_DENOM
}
