package coingeckopricing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

// newTestBatcher returns a batcher whose refresh stops when the test ends.
func newTestBatcher(t *testing.T, url string, refreshInterval time.Duration) *priceBatcher {
	b := newPriceBatcher(url, "usd", refreshInterval)
	t.Cleanup(b.stop)
	return b
}

// newTestServer serves status, or when it is 200 the body, and counts requests.
func newTestServer(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

// setFreshPrice stores a price for id that stays servable for a minute.
func setFreshPrice(b *priceBatcher, id string, price int64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cache[id] = cachedPrice{price: osmomath.NewBigDec(price), expiresAt: time.Now().Add(time.Minute)}
}

// An older request that finishes after a newer one must not overwrite the newer result.
func TestFetchChunk_OlderResponseDoesNotOverwriteNewer(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := map[string]map[string]float64{}
		if requests.Add(1) == 1 {
			// The first (older) request is slow and carries the old price.
			time.Sleep(300 * time.Millisecond)
			result["atom"] = map[string]float64{"usd": 1}
		} else {
			// The second (newer) request is fast and carries the new price.
			result["atom"] = map[string]float64{"usd": 2}
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(server.Close)

	b := newTestBatcher(t, server.URL, time.Hour)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerOnDemand)
	}()
	require.Eventually(t, func() bool { return requests.Load() == 1 }, time.Second, time.Millisecond)

	b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerRefresh)
	wg.Wait()

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, osmomath.NewBigDec(2), b.cache["atom"].price)
}

func TestClampRefreshInterval(t *testing.T) {
	lo, hi := 10*time.Second, 10*time.Minute
	require.Equal(t, lo, clampRefreshInterval(1, lo, hi))
	require.Equal(t, 5*time.Minute, clampRefreshInterval(300000, lo, hi))
	require.Equal(t, hi, clampRefreshInterval(3600000, lo, hi))
	// Large enough to overflow time.Duration if converted before capping.
	require.Equal(t, hi, clampRefreshInterval(9223372036855, lo, hi))
}

// A chunk that is not sent because of a backoff leaves cached prices to expire on their own.
func TestFetchChunk_BackoffKeepsCachedPrices(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
	}))
	t.Cleanup(server.Close)

	b := newTestBatcher(t, server.URL, time.Hour)
	b.mu.Lock()
	b.cache["atom"] = cachedPrice{price: osmomath.NewBigDec(5), expiresAt: time.Now().Add(time.Minute)}
	b.backoffUntil = time.Now().Add(time.Minute)
	b.mu.Unlock()

	b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerRefresh)

	require.Equal(t, int32(0), requests.Load())
	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, osmomath.NewBigDec(5), b.cache["atom"].price)
}

// A request that fails removes the price before its lifetime ends.
func TestFetchChunk_FailedRequestRemovesPrice(t *testing.T) {
	server, requests := newTestServer(t, http.StatusInternalServerError, "")
	b := newTestBatcher(t, server.URL, time.Hour)
	setFreshPrice(b, "atom", 5)

	b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerRefresh)

	require.Equal(t, int32(1), requests.Load())
	b.mu.Lock()
	defer b.mu.Unlock()
	require.NotContains(t, b.cache, "atom")
}

// A 429 says nothing about the prices: they are kept to expire on their own, and the backoff starts.
func TestFetchChunk_RateLimitKeepsCachedPrices(t *testing.T) {
	server, requests := newTestServer(t, http.StatusTooManyRequests, "")
	b := newTestBatcher(t, server.URL, time.Hour)
	setFreshPrice(b, "atom", 5)
	setFreshPrice(b, "osmo", 1)

	require.True(t, b.fetchChunk([]string{"atom", "osmo"}, domain.CoingeckoTriggerRefresh), "a 429 is a rate limit")

	require.Equal(t, int32(1), requests.Load())
	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, osmomath.NewBigDec(5), b.cache["atom"].price)
	require.Equal(t, osmomath.NewBigDec(1), b.cache["osmo"].price)
	require.True(t, time.Now().Before(b.backoffUntil))
}

// An id missing from an answer keeps a price that is still servable and is not marked unpriced;
// an id without a stored price is.
func TestFetchChunk_MissingIdKeepsServablePrice(t *testing.T) {
	server, _ := newTestServer(t, http.StatusOK, `{"osmo":{"usd":1}}`)
	b := newTestBatcher(t, server.URL, time.Hour)
	setFreshPrice(b, "atom", 5)

	b.fetchChunk([]string{"atom", "osmo", "unknown"}, domain.CoingeckoTriggerRefresh)

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, osmomath.NewBigDec(5), b.cache["atom"].price)
	require.NotContains(t, b.noPrice, "atom")
	require.Contains(t, b.noPrice, "unknown")
}

// An id missing from an answer after its price expired is not marked unpriced if it had a price
// within noPriceCacheTTL; the expired price is not served.
func TestFetchChunk_MissingIdRecentlyPricedIsNotNoPrice(t *testing.T) {
	server, _ := newTestServer(t, http.StatusOK, `{"osmo":{"usd":1}}`)
	b := newTestBatcher(t, server.URL, time.Hour)
	call := &priceCall{done: make(chan struct{})}
	b.mu.Lock()
	b.cache["atom"] = cachedPrice{price: osmomath.NewBigDec(5), expiresAt: time.Now().Add(-time.Second)}
	b.pricedAt["atom"] = time.Now().Add(-2 * time.Minute)
	b.pending["atom"] = call
	b.mu.Unlock()

	b.fetchChunk([]string{"atom", "osmo"}, domain.CoingeckoTriggerOnDemand)

	<-call.done
	require.True(t, domain.IsNoPrice(call.err))
	b.mu.Lock()
	defer b.mu.Unlock()
	require.NotContains(t, b.noPrice, "atom")
}

// A refresh cut short by a 429 is retried once the backoff ends, not an interval later.
func TestRefresh_RetriedAfterRateLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"atom":{"usd":5}}`))
	}))
	t.Cleanup(server.Close)
	b := newTestBatcher(t, server.URL, time.Hour)

	b.mu.Lock()
	b.tracked["atom"] = time.Now()
	b.armRefresh(0)
	b.mu.Unlock()

	require.Eventually(t, func() bool { return requests.Load() >= 2 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		_, ok := b.cache["atom"]
		return ok
	}, time.Second, time.Millisecond)
}

// An empty answer to several ids marks none of them unpriced and keeps their prices.
func TestFetchChunk_EmptyAnswerIsNotNoPrice(t *testing.T) {
	server, _ := newTestServer(t, http.StatusOK, `{}`)
	b := newTestBatcher(t, server.URL, time.Hour)
	setFreshPrice(b, "atom", 5)

	// Not a rate limit either, so it does not make the refresh retry early.
	require.False(t, b.fetchChunk([]string{"atom", "osmo"}, domain.CoingeckoTriggerRefresh))

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, osmomath.NewBigDec(5), b.cache["atom"].price)
	require.Empty(t, b.noPrice)
}

// A lookup whose chunk is held back by a backoff gets a price stored meanwhile by another request.
func TestFetchChunk_HeldBackLookupGetsStoredPrice(t *testing.T) {
	server, requests := newTestServer(t, http.StatusOK, `{}`)
	b := newTestBatcher(t, server.URL, time.Hour)
	call := &priceCall{done: make(chan struct{})}
	b.mu.Lock()
	b.pending["atom"] = call
	b.backoffUntil = time.Now().Add(time.Minute)
	b.mu.Unlock()
	setFreshPrice(b, "atom", 5)

	b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerOnDemand)

	require.Equal(t, int32(0), requests.Load())
	<-call.done
	require.NoError(t, call.err)
	require.Equal(t, osmomath.NewBigDec(5), call.price)
}

// Once stopped, the batcher sends no more refreshes.
func TestStop_EndsRefresh(t *testing.T) {
	server, requests := newTestServer(t, http.StatusOK, `{"atom":{"usd":5}}`)
	b := newTestBatcher(t, server.URL, 20*time.Millisecond)

	_, err := b.getPrice(context.Background(), "atom")
	require.NoError(t, err)
	require.Eventually(t, func() bool { return requests.Load() >= 2 }, time.Second, time.Millisecond)

	b.stop()
	stoppedAt := requests.Load()
	time.Sleep(100 * time.Millisecond)
	// At most a refresh already running when stop was called completes.
	require.LessOrEqual(t, requests.Load(), stoppedAt+1)
}

// The next refresh is timed from the start of the cycle, so a slow cycle does not push it back.
func TestRefresh_TimedFromCycleStart(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		// Each request takes most of the interval.
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`{"atom":{"usd":5}}`))
	}))
	t.Cleanup(server.Close)
	b := newTestBatcher(t, server.URL, 400*time.Millisecond)

	b.mu.Lock()
	b.tracked["atom"] = time.Now()
	b.armRefresh(0)
	b.mu.Unlock()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(times) >= 3
	}, 3*time.Second, time.Millisecond)
	b.stop()

	mu.Lock()
	defer mu.Unlock()
	// Timed from the end of each cycle the gap would be about 700ms; from the start it is 400ms.
	gap := times[2].Sub(times[1])
	require.Less(t, gap, 550*time.Millisecond)
}

// A price handed to a waiting lookup that expires before the lookup resumes is not returned.
func TestGetPrice_WaiterDoesNotReturnExpiredPrice(t *testing.T) {
	b := newTestBatcher(t, "http://127.0.0.1:0", time.Hour)
	call := &priceCall{done: make(chan struct{}), price: osmomath.NewBigDec(5), expiresAt: time.Now().Add(-time.Millisecond)}
	close(call.done)
	b.mu.Lock()
	b.pending["atom"] = call
	b.mu.Unlock()

	price, err := b.getPrice(context.Background(), "atom")
	require.Error(t, err)
	require.False(t, domain.IsNoPrice(err))
	require.True(t, price.IsNil())
}
