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
// missed blocks. Skipped blocks are counted in the gap metric. The store is in
// memory: after a restart, ingestion resumes forward from the first block
// signalled and earlier trades are not recovered.
type Ingester struct {
	fetcher          BlockFetcher
	normaliser       *normaliser
	repo             mvc.TradesRepository
	maxCatchUpBlocks int64
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
	logger log.Logger,
) *Ingester {
	return &Ingester{
		fetcher:          fetcher,
		normaliser:       newNormaliser(pools, pricing, usdQuoteDenom, scalingFactor, quoteDenoms),
		repo:             repo,
		maxCatchUpBlocks: int64(maxCatchUpBlocks),
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

	select {
	case i.notify <- struct{}{}:
	default:
		// A wake-up is already pending and will pick up the latest height.
	}
	return nil
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

	if i.lastProcessed == 0 {
		// Forward-only from the first block after startup. Recorded before
		// fetching so that a failed first fetch is retried, not skipped.
		i.lastProcessed = target - 1
	}
	from := i.lastProcessed + 1

	if missed := target - from + 1; missed > i.maxCatchUpBlocks {
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

		if err := i.processHeight(ctx, height); err != nil {
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
			continue
		}

		i.lastProcessed = height
		i.repo.SetLastIngestedHeight(height)
		telemetry.LastHeightGauge.Set(float64(height))
	}
}

func (i *Ingester) processHeight(ctx context.Context, height int64) error {
	ctx, cancel := context.WithTimeout(ctx, blockProcessTimeout)
	defer cancel()

	block, err := i.fetcher.FetchBlock(ctx, height)
	if err != nil {
		return err
	}

	trades, err := i.extractTrades(ctx, block)
	if err != nil {
		return err
	}

	i.repo.StoreTrades(trades, i.now())
	return nil
}

// extractTrades returns the block's trades ascending by key. Only successful
// txs are read: a failed tx's events are discarded by the chain.
func (i *Ingester) extractTrades(ctx context.Context, block BlockData) ([]domain.Trade, error) {
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
			trade, skipReason, err := i.normaliser.normalise(ctx, event, key, block.Time, block.TxHashes[txIndex], prices)
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
