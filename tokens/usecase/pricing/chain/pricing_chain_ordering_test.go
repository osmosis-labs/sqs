package chainpricing

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
)

const orderingTestQuoteDenom = "uquote"

// newOrderingTestPricing returns a chainPricing backed by a router that answers every quote through quoteFn.
func newOrderingTestPricing(quoteFn func(ctx context.Context, baseDenom string) (domain.Quote, []domain.SplitRoute, error)) *chainPricing {
	return &chainPricing{
		TUsecase: &mocks.TokensUsecaseMock{
			GetChainScalingFactorByDenomMutFunc: func(string) (osmomath.Dec, error) {
				return osmomath.OneDec(), nil
			},
		},
		RUsecase: &mocks.RouterUsecaseMock{
			GetSimpleQuoteFunc: func(ctx context.Context, _ sdk.Coin, tokenOutDenom string, _ ...domain.RouterOption) (domain.Quote, []domain.SplitRoute, error) {
				return quoteFn(ctx, tokenOutDenom)
			},
		},
		cache:             cache.New("test"),
		failedCache:       cache.New("test"),
		cacheExpiryNs:     time.Minute,
		defaultQuoteDenom: "udefault",
	}
}

// orderingTestQuote returns a quote with a positive price and a single route.
func orderingTestQuote(baseDenom string) (domain.Quote, []domain.SplitRoute, error) {
	quote := &mocks.MockQuote{
		GetAmountOutFunc: func() sdk.Coin { return sdk.NewCoin(baseDenom, osmomath.NewInt(10)) },
	}
	route := &mocks.RouteMock{
		GetPoolsFunc: func() []domain.RoutablePool { return nil },
	}
	return quote, []domain.SplitRoute{route}, nil
}

// Validates that an older failed computation of a pair does not evict the price of a newer
// successful computation of the same pair, even when computations of many other pairs complete
// in between. Ordering state for a pair must not be evicted while a computation that started
// before the success is still running.
func TestComputePrice_OlderFailureKeepsNewerSuccessUnderCachePressure(t *testing.T) {
	const (
		// More pairs than shards, so that other pairs share the shard of the pair under test.
		otherPairs  = 4 * pairShardCount
		pairBase    = "ubase"
		olderFailed = "older computation failed"
	)

	var pairCalls atomic.Int32
	olderStarted := make(chan struct{})
	releaseOlder := make(chan struct{})

	c := newOrderingTestPricing(func(ctx context.Context, baseDenom string) (domain.Quote, []domain.SplitRoute, error) {
		if baseDenom == pairBase && pairCalls.Add(1) == 1 {
			close(olderStarted)
			<-releaseOlder
			return nil, nil, errors.New(olderFailed)
		}
		return orderingTestQuote(baseDenom)
	})

	olderDone := make(chan error, 1)
	go func() {
		_, _, err := c.computePrice(context.Background(), pairBase, orderingTestQuoteDenom, 0, false)
		olderDone <- err
	}()
	<-olderStarted

	// A newer computation of the pair succeeds while the older one is still running.
	_, _, err := c.computePrice(context.Background(), pairBase, orderingTestQuoteDenom, 0, false)
	require.NoError(t, err)

	// Successes of many other pairs complete while the older computation still runs.
	for i := range otherPairs {
		_, _, err := c.computePrice(context.Background(), fmt.Sprintf("uother%d", i), orderingTestQuoteDenom, 0, false)
		require.NoError(t, err)
	}

	close(releaseOlder)
	require.ErrorContains(t, <-olderDone, olderFailed)

	cacheKey := domain.FormatPricingCacheKey(pairBase, orderingTestQuoteDenom)
	_, found := c.failedCache.Get(cacheKey)
	require.False(t, found, "older failure must not be cached after a newer success")
	_, found = c.cache.Get(cacheKey)
	require.True(t, found, "older failure must not evict the newer success's price")

	// Ordering state is kept only while a computation is in flight.
	require.Zero(t, inFlightPairs(c))
}

// inFlightPairs returns the number of pairs with ordering state across all shards.
func inFlightPairs(c *chainPricing) int {
	n := 0
	for i := range c.pairShards {
		shard := &c.pairShards[i]
		shard.mu.Lock()
		n += len(shard.pairs)
		shard.mu.Unlock()
	}
	return n
}
