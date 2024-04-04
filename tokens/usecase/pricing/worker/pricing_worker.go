package worker

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type PricingWorker interface {
	UpdatePricesAsync(height uint64, baseDenoms map[string]struct{})
}

type pricingWorker struct {
	pricingStrategies map[domain.PricingSource]domain.PricingStrategy
	updateListeners   []PricingUpdateListener
	quoteDenom        string

	// We use this flag to avoid running multiple price updates concurrently
	// as it may cause high load on the system.
	// If an update is missed, cache eviction will trigger it to be recomputed anyways.
	isProcessing atomic.Bool
}

type PricingUpdateListener interface {
	OnPricingUpdate(ctx context.Context, height int64, baseDenomPriceUpdates map[string]osmomath.BigDec, quoteDenom string) error
}

type priceResult struct {
	baseDenom string
	price     osmomath.BigDec
}

func New(pricingStrategies map[domain.PricingSource]domain.PricingStrategy, quoteDenom string, updateListeners []PricingUpdateListener) PricingWorker {
	return &pricingWorker{
		pricingStrategies: pricingStrategies,
		updateListeners:   updateListeners,
		quoteDenom:        quoteDenom,

		isProcessing: atomic.Bool{},
	}
}

// UpdatePrices implements PricingWorker.
func (p *pricingWorker) UpdatePricesAsync(height uint64, baseDenoms map[string]struct{}) {
	if p.isProcessing.Load() {
		return
	}

	p.isProcessing.Store(true)

	go p.updatePrices(height, baseDenoms)
}

func (p *pricingWorker) updatePrices(height uint64, baseDenoms map[string]struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer p.isProcessing.Store(false)
	defer cancel()

	chainStrategy := p.pricingStrategies[domain.ChainPricingSource]

	prices := make(map[string]osmomath.BigDec, len(baseDenoms))

	poolResultChan := make(chan priceResult, len(prices))

	for baseDenom := range baseDenoms {
		go func(baseDenom string) {
			price, err := chainStrategy.ComputePrice(ctx, baseDenom, p.quoteDenom)
			if err != nil {
				price = osmomath.ZeroBigDec()
			}

			poolResultChan <- priceResult{
				baseDenom: baseDenom,
				price:     price,
			}
		}(baseDenom)
	}

	// Acumulate results
	for i := 0; i < len(baseDenoms); i++ {
		select {
		case poolResult := <-poolResultChan:
			prices[poolResult.baseDenom] = poolResult.price
		case <-ctx.Done():
			return // Stop processing
		}
	}

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
