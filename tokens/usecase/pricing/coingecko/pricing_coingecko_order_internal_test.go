package coingeckopricing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestRefreshInterval(t *testing.T) {
	// Tests lower the minimum (see export_test.go); check the clamp against the production one.
	testMin := minRefreshInterval
	minRefreshInterval = 10 * time.Second
	t.Cleanup(func() { minRefreshInterval = testMin })

	require.Equal(t, defaultRefreshInterval, refreshInterval(0))
	require.Equal(t, defaultRefreshInterval, refreshInterval(-1))
	require.Equal(t, 10*time.Second, refreshInterval(1))
	require.Equal(t, 5*time.Minute, refreshInterval(300000))
	require.Equal(t, maxRefreshInterval, refreshInterval(3600000))
	// Large enough to overflow time.Duration if converted before capping.
	require.Equal(t, maxRefreshInterval, refreshInterval(9223372036855))
}

// A request that fails removes the price before its lifetime ends, and the next lookup gets a
// pricing error rather than a no-price answer.
func TestFetchChunk_FailedRequestRemovesPrice(t *testing.T) {
	server, requests := newTestServer(t, http.StatusInternalServerError, "")
	b := newTestBatcher(t, server.URL, time.Hour)
	setFreshPrice(b, "atom", 5)

	require.False(t, b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerRefresh), "a 500 is not a rate limit")

	require.Equal(t, int32(1), requests.Load())
	b.mu.Lock()
	require.NotContains(t, b.cache, "atom")
	b.mu.Unlock()

	price, err := b.getPrice(context.Background(), "atom")
	require.Error(t, err)
	require.False(t, domain.IsNoPrice(err))
	require.True(t, price.IsNil())
	require.Equal(t, int32(2), requests.Load())
}

// A request that cannot be built is a failure, not a rate limit: it removes the price.
func TestFetchChunk_UnbuildableRequestIsFailure(t *testing.T) {
	b := newTestBatcher(t, "http://%zz", time.Hour)
	setFreshPrice(b, "atom", 5)

	require.False(t, b.fetchChunk([]string{"atom"}, domain.CoingeckoTriggerRefresh))

	b.mu.Lock()
	defer b.mu.Unlock()
	require.NotContains(t, b.cache, "atom")
	require.True(t, b.backoffUntil.IsZero())
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

	require.False(t, b.fetchChunk([]string{"atom", "osmo", "unknown"}, domain.CoingeckoTriggerRefresh), "an answer is not a rate limit")

	b.mu.Lock()
	defer b.mu.Unlock()
	require.Equal(t, osmomath.NewBigDec(5), b.cache["atom"].price)
	require.NotContains(t, b.noPrice, "atom")
	require.Contains(t, b.noPrice, "unknown")
}

// An id missing from an answer after its price expired is answered as unpriced only for
// missingPriceRecheck: lookups meanwhile send no request, and the expired price is not served.
func TestFetchChunk_MissingIdWithExpiredPriceIsRecheckedSoon(t *testing.T) {
	server, requests := newTestServer(t, http.StatusOK, `{"osmo":{"usd":1}}`)
	b := newTestBatcher(t, server.URL, time.Hour)
	call := &priceCall{done: make(chan struct{})}
	b.mu.Lock()
	b.cache["atom"] = cachedPrice{price: osmomath.NewBigDec(5), expiresAt: time.Now().Add(-time.Second)}
	b.pending["atom"] = call
	b.mu.Unlock()

	started := time.Now()
	b.fetchChunk([]string{"atom", "osmo"}, domain.CoingeckoTriggerOnDemand)

	<-call.done
	_, err := b.answer("atom", call.err)
	require.True(t, domain.IsNoPrice(err))
	b.mu.Lock()
	until := b.noPrice["atom"]
	b.mu.Unlock()
	require.True(t, until.After(started))
	require.False(t, until.After(time.Now().Add(missingPriceRecheck)))

	_, err = b.getPrice(context.Background(), "atom")
	require.True(t, domain.IsNoPrice(err))
	require.Equal(t, int32(1), requests.Load())
}

func TestParseRetryAfter(t *testing.T) {
	require.Equal(t, 5*time.Second, parseRetryAfter("5"))
	require.Equal(t, maxRateLimitBackoff, parseRetryAfter("3600"))
	// A delay that is missing, unparsable or not in the future backs off for the default.
	for _, value := range []string{"", "soon", "0", "-3", time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)} {
		require.Equal(t, defaultRateLimitBackoff, parseRetryAfter(value), value)
	}
}

// A refresh whose second chunk is rate limited resumes from that chunk once the backoff ends,
// without sending the first chunk again.
func TestRefresh_ResumesFromRateLimitedChunk(t *testing.T) {
	var (
		mu       sync.Mutex
		idCounts []int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		idCounts = append(idCounts, len(strings.Split(r.URL.Query().Get("ids"), ",")))
		n := len(idCounts)
		mu.Unlock()
		if n == 2 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	b := newTestBatcher(t, server.URL, time.Hour)

	b.mu.Lock()
	for i := 0; i <= maxIdsPerRequest; i++ {
		b.tracked[fmt.Sprintf("token-%03d", i)] = time.Now()
	}
	b.startRefresh(0)
	b.mu.Unlock()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(idCounts) >= 3
	}, 5*time.Second, 10*time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	require.Equal(t, []int{maxIdsPerRequest, 1, 1}, idCounts)
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
	price, err := b.answer("atom", call.err)
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)
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

// The next refresh is timed from the start of the cycle, so a slow cycle does not push it back,
// and a cycle that overran still leaves a tenth of the interval before the next.
func TestNextRefreshDelay(t *testing.T) {
	interval := 5 * time.Minute
	require.Equal(t, interval, nextRefreshDelay(interval, 0))
	require.Equal(t, 4*time.Minute, nextRefreshDelay(interval, time.Minute))
	require.Equal(t, interval/10, nextRefreshDelay(interval, interval))
	require.Equal(t, interval/10, nextRefreshDelay(interval, 2*interval))
}

// A lookup released by its request does not return a price that expired before it resumed.
func TestAnswer_DoesNotReturnExpiredPrice(t *testing.T) {
	b := newTestBatcher(t, "http://127.0.0.1:0", time.Hour)
	b.mu.Lock()
	b.cache["atom"] = cachedPrice{price: osmomath.NewBigDec(5), expiresAt: time.Now().Add(-time.Millisecond)}
	b.mu.Unlock()

	price, err := b.answer("atom", nil)
	require.Error(t, err)
	require.False(t, domain.IsNoPrice(err))
	require.True(t, price.IsNil())
}
