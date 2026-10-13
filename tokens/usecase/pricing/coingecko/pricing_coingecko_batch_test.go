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
	tokensUsecase := &mocks.TokensUsecaseMock{
		GetChainDenomFunc: func(humanDenom string) (string, error) {
			return "", errors.New("not found")
		},
		GetCoingeckoIdByChainDenomFunc: func(chainDenom string) (string, error) {
			return chainDenom, nil
		},
	}

	config := domain.PricingConfig{
		CacheExpiryMs:          2000,
		CoingeckoUrl:           url,
		CoingeckoQuoteCurrency: "usd",
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
