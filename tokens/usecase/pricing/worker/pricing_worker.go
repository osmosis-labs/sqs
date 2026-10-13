package worker

import (
	"context"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/log"
	"go.uber.org/zap"
)

type pricingWorker struct {
	updateListeners []domain.PricingUpdateListener
	quoteDenom      string

	tokensUseCase   mvc.TokensUsecase
	minLiquidityCap uint64

	logger log.Logger
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

// UpdatePrices implements PricingWorker.
func (p *pricingWorker) UpdatePricesAsync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	go p.UpdatePricesSync(ctx, height, uniqueBlockPoolMetaData)
}

func (p *pricingWorker) UpdatePricesSync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
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
