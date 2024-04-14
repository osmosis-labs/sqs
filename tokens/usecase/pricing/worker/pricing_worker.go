package worker

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

type PricingWorker interface {
	UpdatePricesAsync(height uint64, baseDenoms map[string]struct{})
}

type pricingWorker struct {
	pricingStrategies map[domain.PricingSourceType]domain.PricingSource
	updateListeners   []PricingUpdateListener
	quoteDenom        string

	// We use this flag to avoid running multiple price updates concurrently
	// as it may cause high load on the system.
	// If an update is missed, cache eviction will trigger it to be recomputed anyways.
	isProcessing atomic.Bool

	priceUpdateBaseDenomMap map[string]struct{}

	tokensUseCase mvc.TokensUsecase

	logger log.Logger
}

type PricingUpdateListener interface {
	OnPricingUpdate(ctx context.Context, height int64, pricesBaseQuoteDenomMap map[string]map[string]any, quoteDenom string) error
}

type priceResult struct {
	baseDenom string
	price     osmomath.BigDec
}

func New(tokensUseCase mvc.TokensUsecase, quoteDenom string, updateListeners []PricingUpdateListener, logger log.Logger) PricingWorker {
	return &pricingWorker{
		updateListeners: updateListeners,
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

	start := time.Now()

	p.logger.Info("starting pricing pre-computation", zap.Uint64("height", height), zap.Int("num_base_denoms", len(baseDenoms)))

	// TODO: add telemetry for cancelation
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer p.isProcessing.Store(false)
	defer cancel()

	// Note that we recompute prices entirely.
	prices, err := p.tokensUseCase.GetPrices(ctx, baseDenoms, []string{p.quoteDenom}, domain.WithRecomputePrices())
	if err != nil {
		// TODO: telemetry and skip silently
	}

	p.logger.Info("pricing pre-computation completed", zap.Uint64("height", height), zap.Duration("duration", time.Since(start)))

	// Update listeners
	for _, listener := range p.updateListeners {
		// Ignore errors
		_ = listener.OnPricingUpdate(ctx, int64(height), prices, p.quoteDenom)
	}

}

// Stop implements PricingWorker.
func (p *pricingWorker) Stop() {
	panic("unimplemented")
}

// Generic function to extract keys from any map.
func keysFromMap[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m)) // Pre-allocate slice with capacity equal to map size
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
