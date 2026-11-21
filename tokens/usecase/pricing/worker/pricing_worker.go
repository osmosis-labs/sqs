package worker

import (
	"context"
	"maps"
	"sync"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/domain/recovery"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

type pricingWorker struct {
	updateListeners []domain.PricingUpdateListener
	quoteDenom      string

	tokensUseCase   mvc.TokensUsecase
	minLiquidityCap uint64

	logger log.Logger

	// computeMu makes pricing runs exclusive: the synchronous first block run and the
	// background runs never price at the same time.
	computeMu sync.Mutex

	// pendingMu guards pending and draining.
	pendingMu sync.Mutex
	// pending is the update waiting for the background run, nil if there is none.
	// Updates that arrive while a run is in progress are merged into it, so at most
	// one run is in progress and one waits, however fast blocks arrive.
	pending *pendingPriceUpdate
	// draining is true while a goroutine runs pending updates.
	draining bool
}

// pendingPriceUpdate is a price update waiting to run.
type pendingPriceUpdate struct {
	ctx      context.Context
	height   uint64
	metadata domain.BlockPoolMetadata
}

const (
	priceUpdateTimeout = time.Minute * 2
)

func New(tokensUseCase mvc.TokensUsecase, quoteDenom string, minLiquidityCap uint64, logger log.Logger) domain.PricingWorker {
	return &pricingWorker{
		updateListeners: []domain.PricingUpdateListener{},
		quoteDenom:      quoteDenom,
		tokensUseCase:   tokensUseCase,
		minLiquidityCap: minLiquidityCap,

		logger: logger,
	}
}

// UpdatePricesAsync implements PricingWorker.
//
// The update runs in the background after any run in progress. If an update is
// already waiting, this one is merged into it instead: the waiting update then
// prices the denoms and pools of both, at the later height. A node catching up
// sends blocks faster than they can be priced, and pricing each one would start
// runs faster than they finish.
func (p *pricingWorker) UpdatePricesAsync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()

	if p.pending == nil {
		p.pending = &pendingPriceUpdate{ctx: ctx, height: height, metadata: cloneBlockPoolMetadata(uniqueBlockPoolMetaData)}
	} else {
		p.pending.merge(ctx, height, uniqueBlockPoolMetaData)
		domain.SQSPricingWorkerCoalescedUpdatesCounter.Inc()
	}

	if p.draining {
		return
	}
	p.draining = true

	go p.drainPending()
}

// drainPending runs pending updates until none is left.
func (p *pricingWorker) drainPending() {
	for {
		p.pendingMu.Lock()
		update := p.pending
		p.pending = nil
		if update == nil {
			p.draining = false
			p.pendingMu.Unlock()
			return
		}
		p.pendingMu.Unlock()

		p.UpdatePricesSync(update.ctx, update.height, update.metadata)
	}
}

// merge adds a later update to the pending one.
// The denoms and pools to reprice are the union of both. The liquidity map and
// the context are taken from the later update: the liquidity map is a snapshot
// of all denoms, not a per block delta.
func (u *pendingPriceUpdate) merge(ctx context.Context, height uint64, later domain.BlockPoolMetadata) {
	if height >= u.height {
		u.ctx = ctx
		u.height = height
		u.metadata.DenomPoolLiquidityMap = later.DenomPoolLiquidityMap
	}

	if u.metadata.UpdatedDenoms == nil && len(later.UpdatedDenoms) > 0 {
		u.metadata.UpdatedDenoms = make(map[string]struct{}, len(later.UpdatedDenoms))
	}
	maps.Copy(u.metadata.UpdatedDenoms, later.UpdatedDenoms)

	if u.metadata.PoolIDs == nil && len(later.PoolIDs) > 0 {
		u.metadata.PoolIDs = make(map[uint64]struct{}, len(later.PoolIDs))
	}
	maps.Copy(u.metadata.PoolIDs, later.PoolIDs)

	u.metadata.IsFirstBlock = u.metadata.IsFirstBlock || later.IsFirstBlock
}

// cloneBlockPoolMetadata copies the sets a merge writes to, so that merging never
// changes the caller's metadata.
func cloneBlockPoolMetadata(metadata domain.BlockPoolMetadata) domain.BlockPoolMetadata {
	metadata.UpdatedDenoms = maps.Clone(metadata.UpdatedDenoms)
	metadata.PoolIDs = maps.Clone(metadata.PoolIDs)
	return metadata
}

// UpdatePricesSync implements PricingWorker.
// It waits for any run in progress, so runs never overlap.
func (p *pricingWorker) UpdatePricesSync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	p.computeMu.Lock()
	defer p.computeMu.Unlock()

	// Nothing above this recovers a panic: UpdatePricesAsync runs it in its own goroutine, and
	// ingest calls it directly for the first block. Without this, a panic in pricing or in a
	// listener would take the whole process down. The update is counted as failed instead.
	defer recovery.Log(p.logger, "recovered panic in pricing pre-computation", domain.SQSPricingWorkerComputeErrorCounter, zap.Uint64("height", height))

	baseDenoms := domain.KeysFromMap(uniqueBlockPoolMetaData.UpdatedDenoms)

	ctx, cancel := context.WithTimeout(ctx, priceUpdateTimeout)
	start := time.Now()
	defer func() {
		// Cancel the context
		cancel()

		p.logger.Info("pricing pre-computation completed", zap.Uint64("height", height), zap.Int("num_base_denoms", len(baseDenoms)), zap.Duration("duration", time.Since(start)))
	}()

	p.logger.Info("starting pricing pre-computation", zap.Uint64("height", height), zap.Int("num_base_denoms", len(baseDenoms)))

	// Note that we recompute prices entirely.
	// The worker's min liquidity floor decides which pools can set the served default-quote prices,
	// so it should match the pricing floor. Tokens whose only pools fall below it fall back to CoinGecko.
	prices, err := p.tokensUseCase.GetPrices(ctx, baseDenoms, []string{p.quoteDenom}, domain.ChainPricingSourceType, domain.WithRecomputePrices(), domain.WithMinPricingPoolLiquidityCap(p.minLiquidityCap))
	if err != nil {
		// Increase error counter
		p.logger.Error(domain.SQSPricingWorkerComputeDurationMetricName, zap.Error(err), zap.Uint64("height", height))
		domain.SQSPricingWorkerComputeErrorCounter.Inc()

		// Do not notify listeners so that the price update height stays behind
		// and the healthcheck reports stale prices.
		return
	}

	// Update listeners
	for _, listener := range p.updateListeners {
		// Ignore errors
		_ = listener.OnPricingUpdate(ctx, height, uniqueBlockPoolMetaData, prices, p.quoteDenom)
	}

	// Measure duration
	domain.SQSPricingWorkerComputeDurationGauge.Set(float64(time.Since(start).Milliseconds()))
}

// RegisterListener implements PricingWorker.
func (p *pricingWorker) RegisterListener(listener domain.PricingUpdateListener) {
	p.updateListeners = append(p.updateListeners, listener)
}
