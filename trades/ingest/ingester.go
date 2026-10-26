package ingest

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/trades/telemetry"
)

const (
	// expiryInterval is how often windows are expired when no block arrives,
	// so aggregates keep rolling while the chain or the ingester is stalled.
	expiryInterval = 10 * time.Second
	// blockProcessTimeout bounds the fetch and pricing work for one block.
	blockProcessTimeout = 15 * time.Second
	// maxHeightAttempts is how many times one height is tried before it is
	// skipped as a gap, so a block that fails every time (e.g. its results
	// were pruned from the node) does not stop ingestion.
	maxHeightAttempts = 3
	// backfillBatchBlocks is how many blocks one catch-up pass reads while
	// backfilling before yielding, so window expiry keeps running.
	backfillBatchBlocks = 500
	// backfillPruneMargin keeps the backfill clear of the node's earliest
	// block, which moves forward as the node prunes.
	backfillPruneMargin = 1000
	// maxBackfillSkips is how many consecutive blocks the backfill may skip
	// before it gives up and ingestion goes live. A failed block is retried on
	// the next signal, so a node that cannot serve old blocks (no block results
	// kept, or pruned) would otherwise keep the backfill behind the chain, and
	// the endpoints unavailable, indefinitely.
	maxBackfillSkips = 10
	// maxStatusAttempts is how many signals the node's earliest block is asked
	// for before the ingester starts without backfill.
	maxStatusAttempts = 3
)

// Ingester ingests trades from token_swapped events.
//
// The node's gRPC ingest push carries pool state but no tx events, so the
// ingester registers as an end-block plugin only to learn which height SQS
// has just ingested, then pulls that block's txs and results from the node
// RPC in its own goroutine. ProcessEndBlock never blocks the ingest path.
//
// Heights are processed strictly in order. A failed fetch is retried on the
// next signal from the same height, up to maxHeightAttempts times before the
// height is skipped, and a lag of more than maxCatchUpBlocks skips the oldest
// missed blocks. Skipped blocks are counted in the gap metric.
//
// The store is in memory. With backfillBlocks set, the ingester first reads up
// to that many blocks before the first signalled block, as far back as the
// node keeps them, so a restart does not empty the store. A trade read more
// than maxCatchUpBlocks behind the chain is not valued at the current spot
// price, which is no measure of it, but at its time from the prices the
// backfilled trades before it imply. The catch-up bound does not apply until the
// backfill has caught up with the chain, and until then the last ingested
// height stays unset, so the endpoints keep answering that trades are not
// ingested yet rather than serving a partial store. If the backfill skips
// maxBackfillSkips blocks in a row, it ends and ingestion goes live, with the
// blocks it did not read counted as a gap. Without backfill, ingestion resumes
// forward from the first block signalled.
type Ingester struct {
	fetcher          BlockFetcher
	normaliser       *normaliser
	repo             mvc.TradesRepository
	maxCatchUpBlocks int64
	backfillBlocks   int64
	logger           log.Logger
	now              func() time.Time

	// latestHeight is the newest height SQS has ingested, written by ProcessEndBlock.
	latestHeight atomic.Int64
	notify       chan struct{}

	// lastProcessed, failedHeight and failedAttempts are only accessed by the Run goroutine.
	lastProcessed int64
	// failedHeight is the height whose processing last failed, tried failedAttempts times.
	failedHeight   int64
	failedAttempts int
	// backfilling is set from startup until the backfill catches up with the
	// latest signalled height.
	backfilling bool
	// history holds the prices implied by the backfilled trades, while backfilling.
	history *tradeTimePrices
	// backfillSkips counts the blocks skipped in a row while backfilling.
	backfillSkips int
	// statusAttempts counts the failed reads of the node's earliest block at startup.
	statusAttempts int
}

var _ domain.EndBlockProcessPlugin = &Ingester{}

// NewIngester returns a trades ingester. usdQuoteDenom is the pricing default
// quote denom; quoteDenoms rank the quote assets used to classify buy/sell.
func NewIngester(
	fetcher BlockFetcher,
	pools PoolGetter,
	pricing domain.PricingSource,
	usdQuoteDenom string,
	scalingFactor domain.ScalingFactorGetterCb,
	quoteDenoms []string,
	repo mvc.TradesRepository,
	maxCatchUpBlocks int,
	backfillBlocks int,
	logger log.Logger,
) *Ingester {
	return &Ingester{
		fetcher:          fetcher,
		normaliser:       newNormaliser(pools, pricing, usdQuoteDenom, scalingFactor, quoteDenoms),
		repo:             repo,
		maxCatchUpBlocks: int64(maxCatchUpBlocks),
		backfillBlocks:   int64(backfillBlocks),
		logger:           logger,
		now:              time.Now,
		notify:           make(chan struct{}, 1),
	}
}

// ProcessEndBlock implements domain.EndBlockProcessPlugin. It records the
// height and wakes the ingester without waiting for it.
func (i *Ingester) ProcessEndBlock(_ context.Context, blockHeight uint64, _ domain.BlockPoolMetadata) error {
	height := int64(blockHeight)
	for {
		current := i.latestHeight.Load()
		if height <= current || i.latestHeight.CompareAndSwap(current, height) {
			break
		}
	}

	i.wake()
	return nil
}

// wake schedules a catch-up pass without waiting for it.
func (i *Ingester) wake() {
	select {
	case i.notify <- struct{}{}:
	default:
		// A wake-up is already pending and will pick up the latest height.
	}
}

// Run processes signalled heights until ctx is cancelled.
func (i *Ingester) Run(ctx context.Context) {
	ticker := time.NewTicker(expiryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-i.notify:
			i.catchUp(ctx)
		case <-ticker.C:
			i.repo.Expire(i.now())
		}
	}
}

// catchUp processes every height after the last processed one up to the
// latest signalled height, within the catch-up bound.
func (i *Ingester) catchUp(ctx context.Context) {
	target := i.latestHeight.Load()
	if target <= i.lastProcessed {
		return
	}

	if i.lastProcessed == 0 && !i.start(ctx, target) {
		// Retried on the next signal.
		return
	}
	from := i.lastProcessed + 1

	if i.backfilling {
		// Read the backfill in batches, yielding between them.
		target = min(target, from+backfillBatchBlocks-1)
	} else if missed := target - from + 1; missed > i.maxCatchUpBlocks {
		skipped := missed - i.maxCatchUpBlocks
		telemetry.GapBlocksCounter.Add(float64(skipped))
		i.logger.Warn("trades ingester fell behind, skipping blocks",
			zap.Int64("from", from), zap.Int64("to", from+skipped-1), zap.Int64("target", target))
		from += skipped
	}

	for height := from; height <= target; height++ {
		if ctx.Err() != nil {
			return
		}

		// Outside the backfill, and within the catch-up bound of the chain while
		// backfilling, a trade is priced at ingest; further behind it is valued
		// at its time. Live ingestion is always priced, even if the chain moves
		// on past the bound during a pass.
		priced := !i.backfilling || i.latestHeight.Load()-height < i.maxCatchUpBlocks
		if err := i.processHeight(ctx, height, priced); err != nil {
			if ctx.Err() != nil {
				return
			}
			telemetry.BlockFetchErrorCounter.Inc()
			i.logger.Error("trades ingester failed to process block", zap.Int64("height", height), zap.Error(err))

			if height != i.failedHeight {
				i.failedHeight, i.failedAttempts = height, 0
			}
			i.failedAttempts++
			if i.failedAttempts < maxHeightAttempts {
				// Retried from this height on the next signal.
				return
			}

			telemetry.GapBlocksCounter.Inc()
			i.logger.Warn("trades ingester skipping block after repeated failures",
				zap.Int64("height", height), zap.Int("attempts", i.failedAttempts))
			i.failedHeight, i.failedAttempts = 0, 0
			i.lastProcessed = height
			if i.backfilling {
				i.backfillSkips++
				if i.backfillSkips >= maxBackfillSkips {
					i.endBackfill()
					i.logger.Warn("trades ingester backfill cannot read the node's blocks, going live without the rest of it",
						zap.Int64("height", height), zap.Int("skipped_in_a_row", maxBackfillSkips))
					// The next pass skips the rest of the backfill under the catch-up
					// bound, rather than reading old blocks at the current spot price.
					i.wake()
					return
				}
			}
			continue
		}

		i.backfillSkips = 0
		i.lastProcessed = height
		if !i.backfilling {
			i.repo.SetLastIngestedHeight(height)
		}
		telemetry.LastHeightGauge.Set(float64(height))
	}

	if i.backfilling {
		i.continueBackfill()
	}
}

// start sets where ingestion begins on the first signal, at target, and
// reports whether it did. Without backfill it is forward-only from target.
// With backfill it starts up to backfillBlocks earlier, clear of the oldest
// block the node keeps. If the node's earliest block cannot be read, start
// returns false to be retried on the next signal, and after maxStatusAttempts
// failures starts forward-only. The start is recorded before fetching so that
// a failed first fetch is retried, not skipped.
func (i *Ingester) start(ctx context.Context, target int64) bool {
	if i.backfillBlocks == 0 {
		i.lastProcessed = target - 1
		return true
	}

	statusCtx, cancel := context.WithTimeout(ctx, blockProcessTimeout)
	defer cancel()
	earliest, err := i.fetcher.EarliestHeight(statusCtx)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		i.statusAttempts++
		if i.statusAttempts < maxStatusAttempts {
			i.logger.Warn("trades ingester cannot read the node's earliest block, retrying on the next block", zap.Error(err))
			return false
		}
		i.logger.Warn("trades ingester cannot read the node's earliest block, starting without backfill", zap.Error(err))
		i.lastProcessed = target - 1
		return true
	}

	i.lastProcessed = target - 1
	from := max(target-i.backfillBlocks, earliest+backfillPruneMargin)
	if from >= target {
		return true
	}

	i.lastProcessed = from - 1
	i.backfilling = true
	i.history = newTradeTimePrices(impliedPriceWindow)
	telemetry.BackfillRemainingGauge.Set(float64(target - from))
	i.logger.Info("trades ingester backfilling",
		zap.Int64("from", from), zap.Int64("to", target-1), zap.Int64("node_earliest", earliest))
	return true
}

// continueBackfill ends the backfill once it has caught up with the latest
// signalled height, which makes the store visible, or schedules the next batch.
func (i *Ingester) continueBackfill() {
	remaining := i.latestHeight.Load() - i.lastProcessed
	if remaining > 0 {
		telemetry.BackfillRemainingGauge.Set(float64(remaining))
		i.wake()
		return
	}

	i.endBackfill()
	i.repo.SetLastIngestedHeight(i.lastProcessed)
	i.logger.Info("trades ingester backfill caught up", zap.Int64("height", i.lastProcessed))
}

// endBackfill ends the backfill, after which ingestion is live.
func (i *Ingester) endBackfill() {
	i.backfilling = false
	i.history = nil
	i.backfillSkips = 0
	telemetry.BackfillRemainingGauge.Set(0)
}

// processHeight stores the trades of one block. Without priced they are valued at their time.
func (i *Ingester) processHeight(ctx context.Context, height int64, priced bool) error {
	ctx, cancel := context.WithTimeout(ctx, blockProcessTimeout)
	defer cancel()

	block, err := i.fetcher.FetchBlock(ctx, height)
	if err != nil {
		return err
	}

	trades, err := i.extractTrades(ctx, block, priced)
	if err != nil {
		return err
	}

	i.repo.StoreTrades(trades, i.now())
	return nil
}

// extractTrades returns the block's trades ascending by key. Only successful
// txs are read: a failed tx's events are discarded by the chain.
func (i *Ingester) extractTrades(ctx context.Context, block BlockData, priced bool) ([]domain.Trade, error) {
	if len(block.TxHashes) != len(block.TxResults) {
		return nil, fmt.Errorf("block %d has %d tx hashes but %d tx results", block.Height, len(block.TxHashes), len(block.TxResults))
	}

	var (
		trades []domain.Trade
		prices = make(priceCache)
	)

	for txIndex, result := range block.TxResults {
		if result == nil || result.Code != 0 {
			continue
		}

		for eventIndex, event := range result.Events {
			if event.Type != eventTypeTokenSwapped {
				continue
			}

			key := domain.TradeKey{BlockHeight: block.Height, TxIndex: uint32(txIndex), EventIndex: uint32(eventIndex)}
			trade, skipReason, err := i.normaliser.normalise(ctx, event, key, block.Time, block.TxHashes[txIndex], prices, priced, i.history)
			if skipReason != "" {
				telemetry.SkippedCounter.WithLabelValues(skipReason).Inc()
				if err != nil {
					i.logger.Warn("skipping malformed token_swapped event",
						zap.Int64("height", block.Height), zap.Int("tx_index", txIndex), zap.Int("event_index", eventIndex), zap.Error(err))
				}
				continue
			}

			direction := "none"
			if trade.Direction != nil {
				direction = string(*trade.Direction)
			}
			telemetry.IngestedCounter.WithLabelValues(string(trade.Source), direction).Inc()

			trades = append(trades, trade)
		}
	}

	return trades, nil
}
