package usecase

import (
	"context"
	"runtime/debug"

	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/domain"
	sqsingesttypes "github.com/osmosis-labs/sqs/ingest/types"
)

const (
	// orderbookTickLoadConcurrency bounds how many orderbook pools load their ticks from the
	// node at once. Each load runs paged smart queries; at start-up every orderbook loads in
	// the same block, and unbounded, those queries ran into the query deadline.
	orderbookTickLoadConcurrency = 8

	// maxOrderbookTickRetriesPerBlock is the number of orderbooks without ticks reloaded per block.
	maxOrderbookTickRetriesPerBlock = 8

	// orderbookTickRetryIntervalBlocks is the minimum number of blocks between two reloads of
	// the same orderbook, so that a book that keeps failing is not retried on every block.
	// Candidates are taken in pool ID order, so while more than about
	// maxOrderbookTickRetriesPerBlock * orderbookTickRetryIntervalBlocks books keep failing,
	// the higher IDs wait until the lower ones recover.
	orderbookTickRetryIntervalBlocks = 10
)

// orderbookTickLoad tracks the tick loads of one orderbook pool.
type orderbookTickLoad struct {
	// inFlight is the number of loads running for the pool.
	inFlight int
	// lastRetryHeight is the height of the last retry started for the pool.
	lastRetryHeight uint64
}

// startOrderbookTickLoad loads the pool's ticks in the background, at most orderbookTickLoadConcurrency
// pools at a time. A load for a pool changed in the block (isRetry false) always starts: the
// repository keeps the newest height, so a slower older load cannot overwrite it. A retry
// starts only when no load is running for the pool, its last retry is old enough, and fewer
// than orderbookTickLoadConcurrency loads are queued or running, so retries never queue
// ahead of the loads of pools that just changed.
// It returns whether a load was started.
func (p *ingestUseCase) startOrderbookTickLoad(ctx context.Context, height uint64, pool sqsingesttypes.PoolI, isRetry bool) bool {
	poolID := pool.GetId()

	p.orderbookTickLoadsMu.Lock()
	load := p.orderbookTickLoads[poolID]
	if isRetry {
		if p.orderbookTickLoadsPending >= orderbookTickLoadConcurrency || load.inFlight > 0 ||
			(load.lastRetryHeight != 0 && height < load.lastRetryHeight+orderbookTickRetryIntervalBlocks) {
			p.orderbookTickLoadsMu.Unlock()
			return false
		}
		load.lastRetryHeight = height
	}
	load.inFlight++
	p.orderbookTickLoads[poolID] = load
	p.orderbookTickLoadsPending++
	p.orderbookTickLoadsMu.Unlock()

	go func() {
		defer func() {
			p.orderbookTickLoadsMu.Lock()
			load := p.orderbookTickLoads[poolID]
			load.inFlight--
			p.orderbookTickLoads[poolID] = load
			p.orderbookTickLoadsPending--
			p.orderbookTickLoadsMu.Unlock()
		}()

		p.orderbookTickLoadSem <- struct{}{}
		defer func() { <-p.orderbookTickLoadSem }()

		// ProcessPool stores a pool's ticks only once all of them are loaded, so a recovered
		// panic leaves the pool's previous ticks in place.
		defer func() {
			if r := recover(); r != nil {
				domain.SQSIngestUsecasePanicRecoveredCounter.WithLabelValues("orderbook_ticks").Inc()
				p.logger.Error("recovered panic while loading orderbook ticks", zap.Uint64("pool_id", poolID), zap.Any("panic", r), zap.ByteString("stack", debug.Stack()))
			}
		}()

		if err := p.orderBookUseCase.ProcessPool(ctx, height, pool); err != nil {
			domain.SQSIngestHandlerProcessOrderbookPoolErrorCounter.Inc()
			p.logger.Error(domain.SQSIngestUsecaseProcessOrderbookPoolErrorMetricName, zap.Error(err), zap.Uint64("pool_id", poolID), zap.Bool("retry", isRetry))
		}
	}()

	return true
}

// retryOrderbooksWithoutTicks reloads, in the background, up to maxOrderbookTickRetriesPerBlock
// orderbooks whose ticks have never been stored, and reports how many orderbooks have none.
//
// Ticks are otherwise loaded only for orderbooks changed in a block. After a restart the
// node's full-state block is the only load a quiet orderbook gets, so if that load failed the
// book had no ticks, and its orders were left out of active orders, until it next traded.
func (p *ingestUseCase) retryOrderbooksWithoutTicks(ctx context.Context, height uint64) {
	orderbooks, err := p.poolsUseCase.GetAllOrderbookPoolIDs()
	if err != nil {
		p.logger.Error("failed to get orderbooks for tick retry", zap.Error(err), zap.Uint64("height", height))
		return
	}

	// Retries cover every orderbook, but the health share counts canonical orderbooks only,
	// as those are the books quotes and the app use.
	withoutTicks, started := 0, 0
	canonicalWithoutTicks, canonicalTotal := 0, 0
	for _, orderbook := range orderbooks {
		isCanonical := p.poolsUseCase.IsCanonicalOrderbookPool(orderbook.PoolID)
		if isCanonical {
			canonicalTotal++
		}

		if _, ok := p.orderBookUseCase.GetAllTicks(orderbook.PoolID); ok {
			continue
		}
		withoutTicks++
		if isCanonical {
			canonicalWithoutTicks++
		}

		if started >= maxOrderbookTickRetriesPerBlock {
			continue
		}

		pool, err := p.poolsUseCase.GetPool(orderbook.PoolID)
		if err != nil {
			p.logger.Error("failed to get orderbook pool for tick retry", zap.Error(err), zap.Uint64("pool_id", orderbook.PoolID))
			continue
		}

		if p.startOrderbookTickLoad(ctx, height, pool, true) {
			started++
		}
	}

	domain.SQSIngestUsecaseOrderbooksWithoutTicksGauge.Set(float64(withoutTicks))
	p.chainInfoUseCase.SetOrderbooksWithoutTicks(canonicalWithoutTicks, canonicalTotal)
}
