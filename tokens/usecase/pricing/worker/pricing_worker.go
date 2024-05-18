package worker

import (
	"context"
	"strconv"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

type pricingWorker struct {
	updateListeners []domain.PricingUpdateListener
	quoteDenom      string

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

		logger: logger,
	}
}

// UpdatePrices implements PricingWorker.
func (p *pricingWorker) UpdatePricesAsync(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	go p.updatePrices(height, uniqueBlockPoolMetaData)
}

func (p *pricingWorker) updatePrices(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	baseDenoms := keysFromMap(uniqueBlockPoolMetaData.DenomLiquidityMap)

	ctx, cancel := context.WithTimeout(context.Background(), priceUpdateTimeout)
	start := time.Now()
	defer func() {
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
		_ = listener.OnPricingUpdate(ctx, int64(height), uniqueBlockPoolMetaData, prices, p.quoteDenom)
	}

	// Measure duration
	domain.SQSPricingWorkerComputeDurationGauge.WithLabelValues(strconv.FormatUint(height, 10), strconv.FormatInt(int64(len(baseDenoms)), 10)).Set(float64(time.Since(start).Milliseconds()))
}

// RegisterListener implements PricingWorker.
func (p *pricingWorker) RegisterListener(listener domain.PricingUpdateListener) {
	p.updateListeners = append(p.updateListeners, listener)
}

// Generic function to extract keys from any map.
func keysFromMap[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m)) // Pre-allocate slice with capacity equal to map size
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
