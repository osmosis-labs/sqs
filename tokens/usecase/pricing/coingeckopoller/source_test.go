package coingeckopoller

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
)

const testInterval = time.Minute

// fakeFetch answers with the prices it holds and records the ids of every call.
// err fails every call; failCalls fails the calls with those 1-based numbers.
// onFetch, when set, runs at the start of every call.
type fakeFetch struct {
	mu        sync.Mutex
	prices    map[string]osmomath.BigDec
	calls     [][]string
	err       error
	failCalls map[int]error
	onFetch   func()
}

func (f *fakeFetch) fetch(_ context.Context, ids []string) (map[string]osmomath.BigDec, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.onFetch != nil {
		f.onFetch()
	}
	f.calls = append(f.calls, append([]string(nil), ids...))
	if err := f.failCalls[len(f.calls)]; err != nil {
		return nil, err
	}
	if f.err != nil {
		return nil, f.err
	}
	result := make(map[string]osmomath.BigDec, len(ids))
	for _, id := range ids {
		if price, ok := f.prices[id]; ok {
			result[id] = price.Clone()
		}
	}
	return result, nil
}

func (f *fakeFetch) numCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// testTokens maps chain denoms to Coingecko ids and backs the tokens use case mock.
type testTokens struct {
	mu  sync.Mutex
	ids map[string]string
}

func (tt *testTokens) set(ids map[string]string) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.ids = ids
}

func (tt *testTokens) mock() *mocks.TokensUsecaseMock {
	return &mocks.TokensUsecaseMock{
		GetFullTokenMetadataFunc: func() (map[string]domain.Token, error) {
			tt.mu.Lock()
			defer tt.mu.Unlock()
			result := make(map[string]domain.Token, len(tt.ids))
			for denom, id := range tt.ids {
				result[denom] = domain.Token{CoinMinimalDenom: denom, CoingeckoID: id}
			}
			return result, nil
		},
		GetCoingeckoIdByChainDenomFunc: func(denom string) (string, error) {
			tt.mu.Lock()
			defer tt.mu.Unlock()
			id, ok := tt.ids[denom]
			if !ok {
				return "", fmt.Errorf("denom %s not found", denom)
			}
			return id, nil
		},
	}
}

// testSource is a Source on a fake clock whose sleep advances the clock instead of waiting.
type testSource struct {
	*Source
	tokens *testTokens
	fetch  *fakeFetch

	mu  sync.Mutex
	now time.Time
}

func newTestSource(t *testing.T, ids map[string]string, prices map[string]osmomath.BigDec) *testSource {
	t.Helper()
	ts := &testSource{
		tokens: &testTokens{ids: ids},
		fetch:  &fakeFetch{prices: prices},
		now:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	ts.Source = newSource(ts.tokens.mock(), ts.fetch.fetch, testInterval, &log.NoOpLogger{})
	ts.Source.now = ts.clock
	return ts
}

func (ts *testSource) clock() time.Time {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.now
}

func (ts *testSource) advance(d time.Duration) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.now = ts.now.Add(d)
}

func (ts *testSource) refresh() error { return ts.Source.refresh(context.Background()) }

func (ts *testSource) getPrice(denom string) (osmomath.BigDec, error) {
	price, _, err := ts.GetPrice(context.Background(), denom, "")
	return price, err
}

func requireUnavailable(t *testing.T, err error) {
	t.Helper()
	require.ErrorIs(t, err, errUnavailable)
	require.False(t, domain.IsNoPrice(err), "an outage must not be reported as the token having no price")
}

// makeIds returns n ids that are their own denoms, each priced.
func makeIds(n int) (map[string]string, map[string]osmomath.BigDec) {
	ids := make(map[string]string, n)
	prices := make(map[string]osmomath.BigDec, n)
	for i := range n {
		id := fmt.Sprintf("token-%03d", i)
		ids[id] = id
		prices[id] = osmomath.NewBigDec(int64(i + 1))
	}
	return ids, prices
}

func TestGetPrice_ServesRefreshedPriceAsACopy(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	require.NoError(t, ts.refresh())

	price, err := ts.getPrice("uatom")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)

	// Mutating the returned price does not change the snapshot.
	price.MulMut(osmomath.NewBigDec(100))
	price, err = ts.getPrice("uatom")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)
	// Lookups never fetch.
	require.Equal(t, 1, ts.fetch.numCalls())
}

func TestGetPrice_BeforeFirstRefreshIsUnavailable(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, nil)

	_, err := ts.getPrice("uatom")
	requireUnavailable(t, err)
	require.Zero(t, ts.fetch.numCalls())
}

func TestGetPrice_IdAnsweredWithoutPriceIsNoPrice(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos", "uindex": "ibc-index"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	require.NoError(t, ts.refresh())

	_, err := ts.getPrice("uindex")
	require.True(t, domain.IsNoPrice(err), err)
}

func TestGetPrice_EmptyOrBlankCoingeckoIdIsNoPrice(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos", "unoid": "", "ublank": "  "}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	require.NoError(t, ts.refresh())

	for _, denom := range []string{"unoid", "ublank"} {
		_, err := ts.getPrice(denom)
		require.True(t, domain.IsNoPrice(err), "%s: %v", denom, err)
	}
	// Neither is requested.
	require.Equal(t, [][]string{{"cosmos"}}, ts.fetch.calls)
}

func TestGetPrice_PaddedIdMatchesTheRefreshedId(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": " cosmos\n"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	require.NoError(t, ts.refresh())

	require.Equal(t, [][]string{{"cosmos"}}, ts.fetch.calls)
	price, err := ts.getPrice("uatom")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)
}

func TestGetPrice_UnknownDenomIsError(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, nil)

	_, err := ts.getPrice("unknown")
	require.Error(t, err)
	require.False(t, domain.IsNoPrice(err))
}

func TestRefresh_FailureKeepsPriceUntilMaxAge(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	require.NoError(t, ts.refresh())

	// The next refresh fails: the price is still served.
	ts.advance(testInterval)
	ts.fetch.err = errors.New("502 Bad Gateway")
	require.Error(t, ts.refresh())
	price, err := ts.getPrice("uatom")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)

	// Up to maxAge after the request that fetched it started, and not after.
	ts.advance(ts.maxAge - testInterval)
	_, err = ts.getPrice("uatom")
	require.NoError(t, err)
	ts.advance(time.Nanosecond)
	_, err = ts.getPrice("uatom")
	requireUnavailable(t, err)

	// A refresh that succeeds again serves prices again.
	ts.fetch.err = nil
	require.NoError(t, ts.refresh())
	_, err = ts.getPrice("uatom")
	require.NoError(t, err)
}

func TestRefresh_IdAnsweredWithoutPriceExpiresAsNoPrice(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos", "uosmo": "osmosis"},
		map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5), "osmosis": osmomath.NewBigDec(1)})
	require.NoError(t, ts.refresh())

	// Coingecko stops pricing the id while it still answers.
	ts.advance(testInterval)
	ts.fetch.prices = map[string]osmomath.BigDec{"osmosis": osmomath.NewBigDec(1)}
	require.NoError(t, ts.refresh())

	// The previous price is served until it expires...
	_, err := ts.getPrice("uatom")
	require.NoError(t, err)

	// ...and then the token has no price, since Coingecko answered for it recently.
	ts.advance(ts.maxAge - testInterval + time.Nanosecond)
	_, err = ts.getPrice("uatom")
	require.True(t, domain.IsNoPrice(err), err)
}

func TestRefresh_NewerPriceReplacesOlder(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	require.NoError(t, ts.refresh())

	ts.advance(testInterval)
	ts.fetch.prices = map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(6)}
	require.NoError(t, ts.refresh())

	price, err := ts.getPrice("uatom")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(6), price)
}

func TestRefresh_ChunksUniqueIdsInStableOrder(t *testing.T) {
	ids, prices := makeIds(250)
	// Two denoms share every id, as bridged variants of a token do in the asset list.
	for denom, id := range ids {
		ids["bridged/"+denom] = id
	}
	ts := newTestSource(t, ids, prices)
	// Each chunk is answered at a different time, as it is in production.
	ts.fetch.onFetch = func() { ts.advance(time.Second) }

	require.NoError(t, ts.refresh())
	require.Len(t, ts.fetch.calls, 3)
	for i, size := range []int{100, 100, 50} {
		require.Len(t, ts.fetch.calls[i], size)
	}
	require.Equal(t, "token-000", ts.fetch.calls[0][0])
	require.Equal(t, "token-249", ts.fetch.calls[2][49])

	// The next refresh sends the same chunks, so request URLs repeat.
	require.NoError(t, ts.refresh())
	require.Equal(t, ts.fetch.calls[:3], ts.fetch.calls[3:])
}

func TestRefresh_FailedChunkKeepsItsOlderPricesAndLaterChunksStillRefresh(t *testing.T) {
	ids, prices := makeIds(250)
	ts := newTestSource(t, ids, prices)
	require.NoError(t, ts.refresh())

	// The next refresh fails on its middle chunk only.
	ts.advance(testInterval)
	ts.fetch.failCalls = map[int]error{5: errors.New("timeout")}
	require.Error(t, ts.refresh())
	require.Equal(t, 6, ts.fetch.numCalls(), "a failed chunk does not stop the chunks after it")

	// The failed chunk is served its previous prices until they expire, then reported unavailable.
	ts.advance(ts.maxAge - testInterval + time.Nanosecond)
	_, err := ts.getPrice("token-150")
	requireUnavailable(t, err)
	for _, denom := range []string{"token-000", "token-249"} {
		_, err := ts.getPrice(denom)
		require.NoError(t, err, denom)
	}
}

func TestRefresh_RateLimitStopsAndHeldBackIdsGoFirst(t *testing.T) {
	ids, prices := makeIds(150)
	ts := newTestSource(t, ids, prices)
	ts.fetch.failCalls = map[int]error{2: &rateLimitError{status: "429"}}

	err := ts.refresh()
	var rateLimited *rateLimitError
	require.ErrorAs(t, err, &rateLimited)

	// The chunk before the rate limit is published.
	_, err = ts.getPrice("token-000")
	require.NoError(t, err)

	// The retry sends the held-back ids first, so a limit that allows one request at a time still
	// refreshes every id in turn instead of resending the ones already answered.
	ts.advance(minRateLimitBackoff)
	ts.fetch.failCalls = map[int]error{4: &rateLimitError{status: "429"}}
	require.Error(t, ts.refresh())
	require.Equal(t, ts.fetch.calls[1], ts.fetch.calls[2][:len(ts.fetch.calls[1])])
	_, err = ts.getPrice("token-149")
	require.NoError(t, err)
}

func TestRefresh_DropsIdsNoLongerInMetadata(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos", "uosmo": "osmosis"},
		map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5), "osmosis": osmomath.NewBigDec(1)})
	require.NoError(t, ts.refresh())

	ts.tokens.set(map[string]string{"uatom": "cosmos"})
	require.NoError(t, ts.refresh())

	require.Equal(t, []string{"cosmos"}, ts.fetch.calls[1])
	require.NotContains(t, ts.snap.Load().entries, "osmosis")
}

func TestRefresh_NoIdsIsError(t *testing.T) {
	ts := newTestSource(t, map[string]string{}, nil)

	require.ErrorIs(t, ts.refresh(), errNoIds)
	require.Zero(t, ts.fetch.numCalls())
}

func TestRefresh_ConcurrentLookups(t *testing.T) {
	ids, prices := makeIds(150)
	ts := newTestSource(t, ids, prices)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = ts.getPrice("token-042")
				}
			}
		}()
	}
	for range 20 {
		require.NoError(t, ts.refresh())
		ts.advance(time.Second)
	}
	close(stop)
	wg.Wait()
}

func TestNextBackoff(t *testing.T) {
	fail := errors.New("fail")
	tests := []struct {
		name string
		prev time.Duration
		err  error
		want time.Duration
	}{
		{"success resets", 8 * time.Second, nil, 0},
		{"first failure", 0, fail, minBackoff},
		{"doubles", 4 * time.Second, fail, 8 * time.Second},
		{"capped at the interval", 40 * time.Second, fail, testInterval},
		{"metadata not loaded does not grow", 32 * time.Second, errNoIds, minBackoff},
		{"rate limit without Retry-After waits the minimum", 0, &rateLimitError{}, minRateLimitBackoff},
		{"honours a longer Retry-After", time.Second, &rateLimitError{retryAfter: 30 * time.Second}, 30 * time.Second},
		{"honours a Retry-After past the interval", 0, &rateLimitError{retryAfter: 5 * time.Minute}, 5 * time.Minute},
		{"keeps doubling past a shorter Retry-After", 8 * time.Second, &rateLimitError{retryAfter: 12 * time.Second}, 16 * time.Second},
		{"finds a rate limit among joined errors", 0, errors.Join(fail, &rateLimitError{retryAfter: 20 * time.Second}), 20 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, nextBackoff(tt.prev, tt.err, testInterval))
		})
	}
}

func TestNextRefreshDelay(t *testing.T) {
	require.Equal(t, testInterval, nextRefreshDelay(testInterval, 0))
	require.Equal(t, testInterval-3*time.Second, nextRefreshDelay(testInterval, 3*time.Second))
	require.Equal(t, testInterval/10, nextRefreshDelay(testInterval, 2*testInterval))
}

func TestRefreshInterval(t *testing.T) {
	require.Equal(t, defaultInterval, refreshInterval(0))
	require.Equal(t, defaultInterval, refreshInterval(-1))
	require.Equal(t, minInterval, refreshInterval(1))
	require.Equal(t, 30*time.Second, refreshInterval(30_000))
	require.Equal(t, maxInterval, refreshInterval(int(maxInterval.Milliseconds())))
	require.Equal(t, maxInterval, refreshInterval(1<<62))
}

func TestMaxAge(t *testing.T) {
	// The batcher this replaces served a price for the interval plus a fifth of it.
	require.Equal(t, 6*time.Minute, maxAge(5*time.Minute))
	require.Equal(t, 12*time.Minute, maxAge(maxInterval))
	// Short intervals get at least minMaxAgeGrace past the interval.
	require.Equal(t, 90*time.Second, maxAge(time.Minute))
	require.Equal(t, 40*time.Second, maxAge(minInterval))
}

func TestNew(t *testing.T) {
	const defaultQuote = "ibc/DEFAULT"
	tokens := (&testTokens{ids: map[string]string{"uatom": "cosmos"}}).mock()
	tokens.GetChainDenomFunc = func(humanDenom string) (string, error) {
		require.Equal(t, "usdc", humanDenom)
		return defaultQuote, nil
	}
	config := domain.PricingConfig{
		DefaultQuoteHumanDenom: "usdc",
		CoingeckoUrl:           "https://prices.osmosis.zone/api/v3/simple/price",
		CoingeckoQuoteCurrency: "usd",
	}

	source, err := New(tokens, config, &log.NoOpLogger{})
	require.NoError(t, err)
	require.Equal(t, defaultInterval, source.interval)
	for _, quote := range []string{"", " ", defaultQuote, USDC_DENOM, NOBLE_USDC_DENOM, USDT_DENOM} {
		require.True(t, source.isAllowedQuoteDenom(quote), quote)
	}
	require.False(t, source.isAllowedQuoteDenom("uatom"))
	_, _, err = source.GetPrice(context.Background(), "uatom", "uatom")
	require.Error(t, err)

	invalid := config
	invalid.CoingeckoQuoteCurrency = " "
	_, err = New(tokens, invalid, &log.NoOpLogger{})
	require.Error(t, err)
}

// runUntil runs Run with sleeps that advance the fake clock and are recorded, until it has slept
// n times, and returns the delays.
func runUntil(t *testing.T, ts *testSource, n int) []time.Duration {
	t.Helper()
	var delays []time.Duration
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts.Source.sleep = func(ctx context.Context, d time.Duration) bool {
		delays = append(delays, d)
		ts.advance(d)
		if len(delays) == n {
			cancel()
		}
		return ctx.Err() == nil
	}
	ts.Run(ctx)
	return delays
}

func TestRun_SchedulesFromRefreshStartAndBacksOff(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	// Every request takes 3s; the second and third fail.
	ts.fetch.onFetch = func() { ts.advance(3 * time.Second) }
	ts.fetch.failCalls = map[int]error{2: errors.New("502"), 3: errors.New("502")}

	delays := runUntil(t, ts, 4)
	require.Equal(t, []time.Duration{
		testInterval - 3*time.Second, // one interval after the refresh started
		minBackoff,                   // first failure
		2 * minBackoff,               // second failure doubles
		testInterval - 3*time.Second, // success resets the backoff
	}, delays)
}

func TestRun_RetriesQuicklyUntilMetadataLoads(t *testing.T) {
	ts := newTestSource(t, map[string]string{}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	var sleeps int
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts.Source.sleep = func(ctx context.Context, d time.Duration) bool {
		sleeps++
		if sleeps <= 3 {
			require.Equal(t, minBackoff, d, "the backoff must not grow while metadata is not loaded")
		}
		if sleeps == 3 {
			ts.tokens.set(map[string]string{"uatom": "cosmos"})
		}
		if sleeps == 4 {
			cancel()
		}
		return ctx.Err() == nil
	}
	ts.Run(ctx)

	_, err := ts.getPrice("uatom")
	require.NoError(t, err)
}

func TestRun_StopsWhenCancelledDuringAFetch(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	ts.fetch.onFetch = cancel
	ts.Source.sleep = func(context.Context, time.Duration) bool {
		t.Fatal("Run must not sleep after its context is cancelled")
		return false
	}

	ts.Run(ctx)
	require.Equal(t, 1, ts.fetch.numCalls())
	require.Panics(t, func() { ts.Run(context.Background()) }, "Run may only be called once")
}

func TestSleep(t *testing.T) {
	require.True(t, sleep(context.Background(), time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, sleep(ctx, time.Hour))
}

// runInBackground runs Run with sleeps that advance the fake clock without waiting, and returns a
// function that stops it and waits for it to return.
func runInBackground(t *testing.T, ts *testSource) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ts.Source.sleep = func(ctx context.Context, d time.Duration) bool {
		ts.advance(d)
		return ctx.Err() == nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ts.Run(ctx)
	}()
	return func() {
		cancel()
		<-done
	}
}

// requireIdleAfter waits until Run has made calls requests in total and is waiting for a lookup.
func requireIdleAfter(t *testing.T, ts *testSource, calls int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return ts.idle.Load() && ts.fetch.numCalls() == calls
	}, 5*time.Second, time.Millisecond)
	require.Equal(t, 1.0, testutil.ToFloat64(domain.SQSPricingCoingeckoIdleGauge))
}

func TestRun_PausesWithoutLookupsAndResumesOnOne(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	// One request per refresh, every minute: one at the start and one after each of the 60
	// minutes in the demand window, then a pause.
	refreshesPerWindow := int(demandWindow/testInterval) + 1
	stop := runInBackground(t, ts)
	defer stop()
	requireIdleAfter(t, ts, refreshesPerWindow)

	// Nothing is requested while paused, so the price expires.
	ts.advance(demandWindow)
	require.Equal(t, refreshesPerWindow, ts.fetch.numCalls())
	_, err := ts.getPrice("uatom")
	requireUnavailable(t, err)

	// That lookup resumed refreshes for another demand window.
	requireIdleAfter(t, ts, 2*refreshesPerWindow)
	price, err := ts.getPrice("uatom")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)
}

func TestRun_LookupsKeepItRefreshing(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, map[string]osmomath.BigDec{"cosmos": osmomath.NewBigDec(5)})
	// A lookup during every second refresh, the last during refresh 150, keeps the demand window
	// open until 60 refreshes after that one.
	const refreshes = 150
	var calls int
	ts.fetch.onFetch = func() {
		calls++
		if calls%2 == 0 && calls <= refreshes {
			_, _ = ts.getPrice("uatom")
		}
	}
	stop := runInBackground(t, ts)
	defer stop()

	requireIdleAfter(t, ts, refreshes+int(demandWindow/testInterval))
}

func TestRun_StopsWhenCancelledWhilePaused(t *testing.T) {
	ts := newTestSource(t, map[string]string{"uatom": "cosmos"}, nil)
	stop := runInBackground(t, ts)
	requireIdleAfter(t, ts, int(demandWindow/testInterval)+1)
	stop()
	require.False(t, ts.idle.Load())
	require.Equal(t, 0.0, testutil.ToFloat64(domain.SQSPricingCoingeckoIdleGauge))
}
