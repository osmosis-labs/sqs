package coingeckopricing

import (
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

	b := newPriceBatcher(server.URL, "usd", time.Hour)

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

	b := newPriceBatcher(server.URL, "usd", time.Hour)
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
