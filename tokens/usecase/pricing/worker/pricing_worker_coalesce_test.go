package worker_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
)

const testWait = 5 * time.Second

// errPanic tells the controlled tokens usecase to panic instead of returning.
var errPanic = errors.New("panic")

// recordingPricingListener records the height of every pricing update.
type recordingPricingListener struct {
	mu      sync.Mutex
	heights []uint64
	poolIDs map[uint64]struct{}
}

func newRecordingPricingListener() *recordingPricingListener {
	return &recordingPricingListener{poolIDs: map[uint64]struct{}{}}
}

func (l *recordingPricingListener) OnPricingUpdate(ctx context.Context, height uint64, metadata domain.BlockPoolMetadata, prices domain.PricesResult, quoteDenom string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.heights = append(l.heights, height)
	for id := range metadata.PoolIDs {
		l.poolIDs[id] = struct{}{}
	}
	return nil
}

func (l *recordingPricingListener) snapshot() ([]uint64, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]uint64(nil), l.heights...), len(l.poolIDs)
}

// controlledPricing is a tokens usecase whose GetPrices reports the denoms it was called with,
// then waits for the result to return: nil to succeed, an error to fail, errPanic to panic.
type controlledPricing struct {
	t        *testing.T
	started  chan []string
	results  chan error
	mu       sync.Mutex
	running  int
	maxConcu int
}

func newControlledPricing(t *testing.T) *controlledPricing {
	return &controlledPricing{t: t, started: make(chan []string, 100), results: make(chan error)}
}

func (c *controlledPricing) usecase() *mocks.TokensUsecaseMock {
	return &mocks.TokensUsecaseMock{
		GetPricesFunc: func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
			c.mu.Lock()
			c.running++
			c.maxConcu = max(c.maxConcu, c.running)
			c.mu.Unlock()
			defer func() {
				c.mu.Lock()
				c.running--
				c.mu.Unlock()
			}()

			sorted := append([]string(nil), baseDenoms...)
			sort.Strings(sorted)
			c.started <- sorted

			err := <-c.results
			if errors.Is(err, errPanic) {
				panic("pricing panicked")
			}
			if err != nil {
				return nil, err
			}
			return domain.PricesResult{}, nil
		},
	}
}

// expectRun waits for the next run to start and checks the denoms it prices.
func (c *controlledPricing) expectRun(denoms ...string) {
	c.t.Helper()
	select {
	case got := <-c.started:
		require.Equal(c.t, denoms, got)
	case <-time.After(testWait):
		c.t.Fatalf("timed out waiting for a pricing run for %v", denoms)
	}
}

// finish ends the run in progress with err.
func (c *controlledPricing) finish(err error) {
	c.t.Helper()
	select {
	case c.results <- err:
	case <-time.After(testWait):
		c.t.Fatal("timed out finishing a pricing run")
	}
}

// expectNoRun checks that no run starts for a while.
func (c *controlledPricing) expectNoRun() {
	c.t.Helper()
	select {
	case got := <-c.started:
		c.t.Fatalf("unexpected pricing run for %v", got)
	case <-time.After(100 * time.Millisecond):
	}
}

func (c *controlledPricing) maxConcurrent() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxConcu
}

// eventually waits until the listener has seen the given heights.
func eventuallyHeights(t *testing.T, listener *recordingPricingListener, want []uint64) {
	t.Helper()
	require.Eventually(t, func() bool {
		got, _ := listener.snapshot()
		return len(got) == len(want)
	}, testWait, time.Millisecond)
	got, _ := listener.snapshot()
	require.Equal(t, want, got)
}

func denomsMetadata(denoms ...string) domain.BlockPoolMetadata {
	metadata := domain.BlockPoolMetadata{UpdatedDenoms: map[string]struct{}{}}
	for _, denom := range denoms {
		metadata.UpdatedDenoms[denom] = struct{}{}
	}
	return metadata
}

func newTestWorker(t *testing.T) (domain.PricingWorker, *controlledPricing, *recordingPricingListener) {
	pricing := newControlledPricing(t)
	pricingWorker := worker.New(pricing.usecase(), USDC, 0, &log.NoOpLogger{})
	listener := newRecordingPricingListener()
	pricingWorker.RegisterListener(listener)
	return pricingWorker, pricing, listener
}

// TestUpdatePricesAsync_CoalescesWhileRunning checks that blocks arriving while a pricing run
// is in progress are merged into one waiting run: runs never overlap, every denom and pool is
// still priced, and the heights reported only increase.
func TestUpdatePricesAsync_CoalescesWhileRunning(t *testing.T) {
	pricingWorker, pricing, listener := newTestWorker(t)

	// Block 1 starts a run that does not finish until told to.
	pricingWorker.UpdatePricesAsync(context.Background(), 1, domain.BlockPoolMetadata{
		UpdatedDenoms: map[string]struct{}{"a": {}},
		PoolIDs:       map[uint64]struct{}{1: {}},
	})
	pricing.expectRun("a")

	coalescedBefore := testutil.ToFloat64(domain.SQSPricingWorkerCoalescedUpdatesCounter)

	// Blocks 2 to 50 arrive while block 1 is being priced.
	callerMetadata := make([]domain.BlockPoolMetadata, 0, 49)
	for height := uint64(2); height <= 50; height++ {
		metadata := domain.BlockPoolMetadata{
			UpdatedDenoms: map[string]struct{}{"b": {}},
			PoolIDs:       map[uint64]struct{}{height: {}},
		}
		// Overlapping and disjoint denoms.
		if height%10 == 0 {
			metadata.UpdatedDenoms[string(rune('a'+height/10+1))] = struct{}{}
		}
		callerMetadata = append(callerMetadata, metadata)
		pricingWorker.UpdatePricesAsync(context.Background(), height, metadata)
	}

	// The first of blocks 2 to 50 creates the waiting update and the other 48 merge into it.
	require.Equal(t, float64(48), testutil.ToFloat64(domain.SQSPricingWorkerCoalescedUpdatesCounter)-coalescedBefore)

	// Finish block 1. Exactly one more run follows, for all the merged blocks.
	pricing.finish(nil)
	pricing.expectRun("b", "c", "d", "e", "f", "g")
	pricing.finish(nil)
	pricing.expectNoRun()

	eventuallyHeights(t, listener, []uint64{1, 50})
	_, numPools := listener.snapshot()
	require.Equal(t, 50, numPools)
	require.Equal(t, 1, pricing.maxConcurrent())

	// Merging never changes the metadata the caller passed in.
	for i, metadata := range callerMetadata {
		require.Len(t, metadata.PoolIDs, 1, "metadata of block %d", i+2)
	}
}

// TestUpdatePricesAsync_RetriesFailedRuns checks that the work of a failed run, including one
// that panicked, is priced with the waiting update or with the next block instead of being
// lost, that a failure does not start a run of its own, and that work is dropped after three
// failed runs.
func TestUpdatePricesAsync_RetriesFailedRuns(t *testing.T) {
	pricingWorker, pricing, listener := newTestWorker(t)
	errorsBefore := testutil.ToFloat64(domain.SQSPricingWorkerComputeErrorCounter)

	// Block 1's run panics while block 2 waits: block 1's work joins block 2's.
	pricingWorker.UpdatePricesAsync(context.Background(), 1, denomsMetadata("a"))
	pricing.expectRun("a")
	pricingWorker.UpdatePricesAsync(context.Background(), 2, denomsMetadata("b"))
	pricing.finish(errPanic)
	pricing.expectRun("a", "b")
	require.Equal(t, float64(1), testutil.ToFloat64(domain.SQSPricingWorkerComputeErrorCounter)-errorsBefore)

	// That run fails with nothing waiting: it is not retried on its own, only with block 3.
	pricing.finish(errors.New("deadline exceeded"))
	pricing.expectNoRun()
	pricingWorker.UpdatePricesAsync(context.Background(), 3, denomsMetadata("c"))
	pricing.expectRun("a", "b", "c")

	// The third failure drops a, b and c.
	pricing.finish(errors.New("failed"))
	pricing.expectNoRun()
	pricingWorker.UpdatePricesAsync(context.Background(), 4, denomsMetadata("d"))
	pricing.expectRun("d")
	pricing.finish(nil)

	eventuallyHeights(t, listener, []uint64{4})
}

// TestUpdatePricesSync_WaitsForRunningUpdate checks that the synchronous run does not overlap a
// background run, and that the first block's synchronous run followed by its background run
// still prices twice.
func TestUpdatePricesSync_WaitsForRunningUpdate(t *testing.T) {
	pricingWorker, pricing, listener := newTestWorker(t)

	pricingWorker.UpdatePricesAsync(context.Background(), 1, denomsMetadata("a"))
	pricing.expectRun("a")

	syncDone := make(chan struct{})
	go func() {
		pricingWorker.UpdatePricesSync(context.Background(), 2, denomsMetadata("b"))
		close(syncDone)
	}()

	// The synchronous run waits for the background run.
	pricing.expectNoRun()
	pricing.finish(nil)
	pricing.expectRun("b")
	pricing.finish(nil)

	select {
	case <-syncDone:
	case <-time.After(testWait):
		t.Fatal("timed out waiting for the synchronous run")
	}

	// The first block runs synchronously, then again in the background.
	pricingWorker.UpdatePricesAsync(context.Background(), 2, denomsMetadata("b"))
	pricing.expectRun("b")
	pricing.finish(nil)

	eventuallyHeights(t, listener, []uint64{1, 2, 2})
	require.Equal(t, 1, pricing.maxConcurrent())
}
