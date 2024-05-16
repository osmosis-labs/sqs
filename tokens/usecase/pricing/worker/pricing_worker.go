package worker

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

type pricingWorker struct {
	updateListeners []domain.PricingUpdateListener
	quoteDenom      string

	// We use this flag to avoid running multiple price updates concurrently
	// as it may cause high load on the system.
	// If an update is missed, cache eviction will trigger it to be recomputed anyways.
	isProcessing atomic.Bool

	priceUpdateBaseDenomMap map[string]struct{}

	tokensUseCase mvc.TokensUsecase

	logger log.Logger
}

const (
	priceUpdateTimeout = time.Minute * 2
)

func New(tokensUseCase mvc.TokensUsecase, quoteDenom string, logger log.Logger) domain.PricingWorker {
	return &pricingWorker{
		updateListeners: []domain.PricingUpdateListener{},
		quoteDenom:      quoteDenom,
		tokensUseCase:   tokensUseCase,

		isProcessing: atomic.Bool{},

		priceUpdateBaseDenomMap: make(map[string]struct{}),

		logger: logger,
	}
}

// UpdatePrices implements PricingWorker.
func (p *pricingWorker) UpdatePricesAsync(height uint64, baseDenoms map[string]struct{}) {
	// Queue pricing updates
	for baseDenom := range baseDenoms {
		p.priceUpdateBaseDenomMap[baseDenom] = struct{}{}
	}

	if p.isProcessing.Load() {
		p.logger.Info("pricing update queued", zap.Uint64("height", height))

		return
	}

	p.isProcessing.Store(true)

	// Get all tokens from the queue map

	baseDenomsSlice := keysFromMap(p.priceUpdateBaseDenomMap)

	// Empty the queue
	p.priceUpdateBaseDenomMap = make(map[string]struct{})

	go p.updatePrices(height, baseDenomsSlice)
}

func (p *pricingWorker) updatePrices(height uint64, baseDenoms []string) {
	ctx, cancel := context.WithTimeout(context.Background(), priceUpdateTimeout)
	start := time.Now()
	defer func() {
		// Reset the processing flag
		p.isProcessing.Store(false)

		// Cancel the context
		cancel()

		p.logger.Info("pricing pre-computation completed", zap.Uint64("height", height), zap.Duration("duration", time.Since(start)))
	}()

	p.logger.Info("starting pricing pre-computation", zap.Uint64("height", height), zap.Int("num_base_denoms", len(baseDenoms)))

	// Note that we recompute prices entirely.
	// Min osmo liquidity must be zero. The reason is that some pools have TVL incorrectly calculated as zero.
	// For example, BRNCH / STRDST (1288). As a result, they are incorrectly excluded despite having appropriate liquidity.
	prices, err := p.tokensUseCase.GetPrices(ctx, baseDenoms, []string{p.quoteDenom}, domain.ChainPricingSourceType, domain.WithRecomputePrices(), domain.WithMinLiquidity(0))
	if err != nil {
		p.logger.Error("failed to pre-compute prices", zap.Error(err))

		// Increase error counter
		domain.SQSPricingWorkerComputeErrorCounter.WithLabelValues(strconv.FormatUint(height, 10)).Inc()
	}

	// Update listeners
	for _, listener := range p.updateListeners {
		// Ignore errors
		_ = listener.OnPricingUpdate(ctx, int64(height), prices, p.quoteDenom)
	}

	// Measure duration
	domain.SQSPricingWorkerComputeDurationGauge.Add(float64(time.Since(start).Milliseconds()))
}

// RegisterListener implements PricingWorker.
func (p *pricingWorker) RegisterListener(listener domain.PricingUpdateListener) {
	p.updateListeners = append(p.updateListeners, listener)
}

// IsProcessing implements PricingWorker.
func (p *pricingWorker) IsProcessing() bool {
	return p.isProcessing.Load()
}

// Generic function to extract keys from any map.
func keysFromMap[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m)) // Pre-allocate slice with capacity equal to map size
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
