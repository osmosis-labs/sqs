package coingeckopricing_test

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/osmosis-labs/sqs/domain/mocks"
	coingeckopricing "github.com/osmosis-labs/sqs/tokens/usecase/pricing/coingecko"
)

// fakeCoingecko serves {"<id>": {"usd": <price>}} for every requested id that has a price and counts requests.
type fakeCoingecko struct {
	server   *httptest.Server
	requests atomic.Int32

	mu       sync.Mutex
	idCounts []int
	prices   map[string]float64
	// status, when set, is returned once instead of a price response.
	status     int
	retryAfter string
	// failStatus, when set, is returned instead of every price response until cleared.
	failStatus int
}

func newFakeCoingecko(t *testing.T, prices map[string]float64) *fakeCoingecko {
	f := &fakeCoingecko{prices: prices}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)

		f.mu.Lock()
		defer f.mu.Unlock()

		if f.status != 0 {
			w.Header().Set("Retry-After", f.retryAfter)
			w.WriteHeader(f.status)
			f.status = 0
			return
		}

		if f.failStatus != 0 {
			w.WriteHeader(f.failStatus)
			return
		}

		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		f.idCounts = append(f.idCounts, len(ids))

		result := map[string]map[string]float64{}
		for _, id := range ids {
			if price, ok := f.prices[id]; ok {
				result[id] = map[string]float64{"usd": price}
			}
		}
		_ = json.NewEncoder(w).Encode(result)
	}))
	t.Cleanup(f.server.Close)
	return f
}

// newBatchedPricing returns a Coingecko pricing source using the default (batched) price getter.
// Base denoms map to Coingecko ids of the same name.
func newBatchedPricing(url string) domain.PricingSource {
	return newBatchedPricingWithRefresh(url, 0)
}

// newBatchedPricingWithRefresh is newBatchedPricing with the bulk refresh interval set; zero uses the default.
func newBatchedPricingWithRefresh(url string, refreshInterval time.Duration) domain.PricingSource {
	tokensUsecase := &mocks.TokensUsecaseMock{
		GetChainDenomFunc: func(humanDenom string) (string, error) {
			return "", errors.New("not found")
		},
		GetCoingeckoIdByChainDenomFunc: func(chainDenom string) (string, error) {
			return chainDenom, nil
		},
	}

	config := domain.PricingConfig{
		// Short enough that the outer per-denom cache never hides a batcher result in these tests.
		CacheExpiryMs:              1,
		CoingeckoUrl:               url,
		CoingeckoQuoteCurrency:     "usd",
		CoingeckoRefreshIntervalMs: int(refreshInterval / time.Millisecond),
	}

	return coingeckopricing.New(tokensUsecase, config, nil)
}

// getConcurrently prices all ids at the same time and returns the errors by id.
func getConcurrently(pricing domain.PricingSource, ids []string) map[string]error {
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		start = make(chan struct{})
		errs  = make(map[string]error, len(ids))
	)

	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			_, _, err := pricing.GetPrice(context.Background(), id, "")
			mu.Lock()
			errs[id] = err
			mu.Unlock()
		}(id)
	}

	close(start)
	wg.Wait()
	return errs
}

func makeIds(n int) ([]string, map[string]float64) {
	ids := make([]string, n)
	prices := make(map[string]float64, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("token-%d", i)
		prices[ids[i]] = float64(i + 1)
	}
	return ids, prices
}

func TestGetPrice_ConcurrentCallsShareOneRequest(t *testing.T) {
	ids, prices := makeIds(40)
	fake := newFakeCoingecko(t, prices)
	pricing := newBatchedPricing(fake.server.URL)

	for id, err := range getConcurrently(pricing, ids) {
		require.NoError(t, err, id)
	}
	require.Equal(t, int32(1), fake.requests.Load())

	// Served from cache afterwards, without another request.
	price, _, err := pricing.GetPrice(context.Background(), "token-0", "")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(1), price)
	require.Equal(t, int32(1), fake.requests.Load())
}

func TestGetPrice_ChunksLargeBatches(t *testing.T) {
	ids, prices := makeIds(150)
	fake := newFakeCoingecko(t, prices)
	pricing := newBatchedPricing(fake.server.URL)

	for id, err := range getConcurrently(pricing, ids) {
		require.NoError(t, err, id)
	}

	require.Equal(t, int32(2), fake.requests.Load())
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.ElementsMatch(t, []int{100, 50}, fake.idCounts)
}

func TestGetPrice_RateLimitBackoff(t *testing.T) {
	fake := newFakeCoingecko(t, map[string]float64{"atom": 5})
	fake.status = http.StatusTooManyRequests
	fake.retryAfter = "1"
	pricing := newBatchedPricing(fake.server.URL)

	_, _, err := pricing.GetPrice(context.Background(), "atom", "")
	require.Error(t, err)
	require.False(t, domain.IsNoPrice(err), "a rate limit is a failure of the source, not a token without a price")
	require.Equal(t, int32(1), fake.requests.Load())

	// Within the Retry-After window no request is sent.
	_, _, err = pricing.GetPrice(context.Background(), "atom", "")
	require.Error(t, err)
	require.Equal(t, int32(1), fake.requests.Load())

	time.Sleep(1100 * time.Millisecond)

	price, _, err := pricing.GetPrice(context.Background(), "atom", "")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)
	require.Equal(t, int32(2), fake.requests.Load())
}

func TestGetPrice_ZeroOrMissingPriceIsError(t *testing.T) {
	fake := newFakeCoingecko(t, map[string]float64{"zero": 0, "tiny": 1.5e-9})
	pricing := newBatchedPricing(fake.server.URL)

	errs := getConcurrently(pricing, []string{"zero", "missing", "tiny"})
	require.Error(t, errs["zero"])
	require.Error(t, errs["missing"])
	require.NoError(t, errs["tiny"])

	// A missing price means Coingecko has none for the token; a zero price is an anomaly.
	require.True(t, domain.IsNoPrice(errs["missing"]))
	require.False(t, domain.IsNoPrice(errs["zero"]))

	// Full precision is kept.
	price, _, err := pricing.GetPrice(context.Background(), "tiny", "")
	require.NoError(t, err)
	require.Equal(t, osmomath.MustNewBigDecFromStr("0.0000000015"), price)

	// Failures are not cached: asking again sends a new request.
	requestsBefore := fake.requests.Load()
	_, _, err = pricing.GetPrice(context.Background(), "zero", "")
	require.Error(t, err)
	require.Equal(t, requestsBefore+1, fake.requests.Load())
}

func TestGetPrice_NoPriceIsCached(t *testing.T) {
	fake := newFakeCoingecko(t, map[string]float64{})
	pricing := newBatchedPricing(fake.server.URL)

	_, _, err := pricing.GetPrice(context.Background(), "ibc-index", "")
	require.True(t, domain.IsNoPrice(err))
	require.Equal(t, int32(1), fake.requests.Load())

	// Coingecko has no price for the id: it is answered as unpriced without another request.
	time.Sleep(10 * time.Millisecond)
	_, _, err = pricing.GetPrice(context.Background(), "ibc-index", "")
	require.True(t, domain.IsNoPrice(err))
	require.Equal(t, int32(1), fake.requests.Load())
}

func TestGetPrice_RefreshFetchesTrackedIdsInOneRequest(t *testing.T) {
	ids, prices := makeIds(10)
	fake := newFakeCoingecko(t, prices)
	// Longer than the ten sequential lookups below (each waits out the batch window).
	pricing := newBatchedPricingWithRefresh(fake.server.URL, time.Second)

	// Lookups spread out in time are fetched one at a time.
	for _, id := range ids {
		_, _, err := pricing.GetPrice(context.Background(), id, "")
		require.NoError(t, err, id)
	}
	onDemand := fake.requests.Load()
	require.Equal(t, int32(len(ids)), onDemand)

	fake.mu.Lock()
	fake.prices["token-0"] = 99
	fake.mu.Unlock()

	// One refresh re-fetches every tracked id in a single request.
	require.Eventually(t, func() bool { return fake.requests.Load() == onDemand+1 }, 3*time.Second, 10*time.Millisecond)
	fake.mu.Lock()
	require.Equal(t, len(ids), fake.idCounts[len(fake.idCounts)-1])
	fake.mu.Unlock()

	// Lookups are then served from the refreshed prices without new requests.
	time.Sleep(10 * time.Millisecond)
	price, _, err := pricing.GetPrice(context.Background(), "token-0", "")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(99), price)
	require.Equal(t, onDemand+1, fake.requests.Load())
}

func TestGetPrice_FailedRefreshLetsPricesExpire(t *testing.T) {
	fake := newFakeCoingecko(t, map[string]float64{"atom": 5})
	pricing := newBatchedPricingWithRefresh(fake.server.URL, 200*time.Millisecond)

	price, _, err := pricing.GetPrice(context.Background(), "atom", "")
	require.NoError(t, err)
	require.Equal(t, osmomath.NewBigDec(5), price)

	fake.mu.Lock()
	fake.failStatus = http.StatusInternalServerError
	fake.mu.Unlock()

	// The refresh at 200ms fails, and the price expires at 240ms (interval plus 20%).
	time.Sleep(300 * time.Millisecond)

	// The old price is not served: the lookup fetches on demand and reports the failure.
	_, _, err = pricing.GetPrice(context.Background(), "atom", "")
	require.Error(t, err)
	require.False(t, domain.IsNoPrice(err), "a failed request is a failure of the source, not a token without a price")
}

func TestGetPrice_ReservedCharacterInIdDoesNotBreakTheBatch(t *testing.T) {
	fake := newFakeCoingecko(t, map[string]float64{"a&b": 3, "atom": 5})
	pricing := newBatchedPricing(fake.server.URL)

	errs := getConcurrently(pricing, []string{"a&b", "atom"})
	require.NoError(t, errs["a&b"])
	require.NoError(t, errs["atom"])
	require.Equal(t, int32(1), fake.requests.Load())
}

func TestGetPrice_RefreshSkipsIdsWithoutPrice(t *testing.T) {
	fake := newFakeCoingecko(t, map[string]float64{"atom": 5})
	pricing := newBatchedPricingWithRefresh(fake.server.URL, 300*time.Millisecond)

	errs := getConcurrently(pricing, []string{"atom", "ibc-index"})
	require.NoError(t, errs["atom"])
	require.True(t, domain.IsNoPrice(errs["ibc-index"]))
	require.Equal(t, int32(1), fake.requests.Load())

	// The refresh asks only for the id that has a price; the no-price answer is still cached.
	require.Eventually(t, func() bool { return fake.requests.Load() == 2 }, 2*time.Second, 10*time.Millisecond)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	require.Equal(t, 1, fake.idCounts[len(fake.idCounts)-1])
}
