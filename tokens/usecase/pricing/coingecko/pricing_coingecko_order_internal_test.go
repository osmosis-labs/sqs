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
