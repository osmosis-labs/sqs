package worker_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
)

// recordingPricingListener records the height and denoms of every pricing update.
type recordingPricingListener struct {
	mu      sync.Mutex
	heights []uint64
	denoms  map[string]struct{}
	poolIDs map[uint64]struct{}
	updates chan struct{}
}

func newRecordingPricingListener() *recordingPricingListener {
	return &recordingPricingListener{
		denoms:  map[string]struct{}{},
		poolIDs: map[uint64]struct{}{},
		updates: make(chan struct{}, 1000),
	}
}

func (l *recordingPricingListener) OnPricingUpdate(ctx context.Context, height uint64, metadata domain.BlockPoolMetadata, prices domain.PricesResult, quoteDenom string) error {
	l.mu.Lock()
	l.heights = append(l.heights, height)
	for denom := range metadata.UpdatedDenoms {
		l.denoms[denom] = struct{}{}
	}
	for id := range metadata.PoolIDs {
		l.poolIDs[id] = struct{}{}
	}
	l.mu.Unlock()
	l.updates <- struct{}{}
	return nil
}

func (l *recordingPricingListener) waitForUpdate(t *testing.T) {
	t.Helper()
	select {
	case <-l.updates:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a pricing update")
	}
}

// blockingTokensUsecase returns a tokens usecase whose GetPrices waits on release
// and reports how many calls run at once.
func blockingTokensUsecase(release <-chan struct{}, started chan<- []string, maxConcurrent *int, mu *sync.Mutex) *mocks.TokensUsecaseMock {
	running := 0
	return &mocks.TokensUsecaseMock{
		GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
			mu.Lock()
			running++
			if running > *maxConcurrent {
				*maxConcurrent = running
			}
			mu.Unlock()

			sorted := append([]string(nil), baseDenoms...)
			sort.Strings(sorted)
			started <- sorted
			<-release

			mu.Lock()
			running--
			mu.Unlock()
			return domain.PricesResult{}, nil
		},
	}
}

// TestUpdatePricesAsync_CoalescesWhileRunning checks that blocks arriving while a
// pricing run is in progress are merged into one waiting run: runs never overlap,
// every denom and pool is still priced, and the heights reported only increase.
func TestUpdatePricesAsync_CoalescesWhileRunning(t *testing.T) {
	release := make(chan struct{})
	started := make(chan []string, 100)
	var mu sync.Mutex
	maxConcurrent := 0

	pricingWorker := worker.New(blockingTokensUsecase(release, started, &maxConcurrent, &mu), USDC, 0, &log.NoOpLogger{})
	listener := newRecordingPricingListener()
	pricingWorker.RegisterListener(listener)

	// Block 1 starts a run that does not finish until released.
	pricingWorker.UpdatePricesAsync(context.Background(), 1, domain.BlockPoolMetadata{
		UpdatedDenoms: map[string]struct{}{"a": {}},
		PoolIDs:       map[uint64]struct{}{1: {}},
	})
	require.Equal(t, []string{"a"}, <-started)

	// Blocks 2 to 50 arrive while block 1 is being priced.
	callerMetadata := make([]domain.BlockPoolMetadata, 0, 49)
	for height := uint64(2); height <= 50; height++ {
		metadata := domain.BlockPoolMetadata{
			UpdatedDenoms: map[string]struct{}{},
			PoolIDs:       map[uint64]struct{}{height: {}},
		}
		// Overlapping and disjoint denoms.
		metadata.UpdatedDenoms["b"] = struct{}{}
		if height%10 == 0 {
			metadata.UpdatedDenoms[string(rune('a'+height/10+1))] = struct{}{}
		}
		callerMetadata = append(callerMetadata, metadata)
		pricingWorker.UpdatePricesAsync(context.Background(), height, metadata)
	}

	// Release block 1. Exactly one more run follows, for all the merged blocks.
	release <- struct{}{}
	listener.waitForUpdate(t)

	merged := <-started
	require.Equal(t, []string{"b", "c", "d", "e", "f", "g"}, merged)
	release <- struct{}{}
	listener.waitForUpdate(t)

	select {
	case denoms := <-started:
		t.Fatalf("unexpected extra pricing run for %v", denoms)
	case <-time.After(100 * time.Millisecond):
	}

	listener.mu.Lock()
	defer listener.mu.Unlock()
	require.Equal(t, []uint64{1, 50}, listener.heights)
	require.Len(t, listener.poolIDs, 50)
	require.Equal(t, 1, maxConcurrent)

	// Merging never changes the metadata the caller passed in.
	for i, metadata := range callerMetadata {
		require.Len(t, metadata.PoolIDs, 1, "metadata of block %d", i+2)
	}
}

// TestUpdatePricesSync_WaitsForRunningUpdate checks that the synchronous run
// does not overlap a background run.
func TestUpdatePricesSync_WaitsForRunningUpdate(t *testing.T) {
	release := make(chan struct{})
	started := make(chan []string, 10)
	var mu sync.Mutex
	maxConcurrent := 0

	pricingWorker := worker.New(blockingTokensUsecase(release, started, &maxConcurrent, &mu), USDC, 0, &log.NoOpLogger{})
	listener := newRecordingPricingListener()
	pricingWorker.RegisterListener(listener)

	pricingWorker.UpdatePricesAsync(context.Background(), 1, domain.BlockPoolMetadata{UpdatedDenoms: map[string]struct{}{"a": {}}})
	<-started

	syncDone := make(chan struct{})
	go func() {
		pricingWorker.UpdatePricesSync(context.Background(), 2, domain.BlockPoolMetadata{UpdatedDenoms: map[string]struct{}{"b": {}}})
		close(syncDone)
	}()

	select {
	case <-started:
		t.Fatal("synchronous run started while a background run was in progress")
	case <-time.After(100 * time.Millisecond):
	}

	release <- struct{}{}
	require.Equal(t, []string{"b"}, <-started)
	release <- struct{}{}
	<-syncDone

	require.Equal(t, 1, maxConcurrent)
}
