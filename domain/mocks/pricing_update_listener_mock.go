package mocks

import (
	"context"
	"time"

	pricingWorker "github.com/osmosis-labs/sqs/tokens/usecase/pricing/worker"
)

type PricingUpdateListenerMock struct {
	Height                 int
	PricesBaseQuteDenomMap map[string]map[string]any
	QuoteDenom             string

	Done chan struct{}

	MockErrorToReturn error

	timeout time.Duration
}

func NewPricingListenerMock(timeout time.Duration) *PricingUpdateListenerMock {
	return &PricingUpdateListenerMock{
		Done:                   make(chan struct{}),
		PricesBaseQuteDenomMap: make(map[string]map[string]any),
		timeout:                timeout,
	}
}

var _ pricingWorker.PricingUpdateListener = &PricingUpdateListenerMock{}

// OnPricingUpdate implements worker.PricingUpdateListener.
func (p *PricingUpdateListenerMock) OnPricingUpdate(ctx context.Context, height int64, pricesBaseQuoteDenomMap map[string]map[string]any, quoteDenom string) error {
	p.Height = int(height)
	p.QuoteDenom = quoteDenom

	// Update the prices map.
	for baseDenom, basePrice := range pricesBaseQuoteDenomMap {
		p.PricesBaseQuteDenomMap[baseDenom] = basePrice
	}

	close(p.Done)

	return p.MockErrorToReturn
}

// Wait blocks until OnPricingUpdate is called.
func (p *PricingUpdateListenerMock) WaitOrTimeout() (didTimeout bool) {
	defer func() {
		// Reset the Done channel.
		p.Done = make(chan struct{})
	}()

	// Wait for 5 seconds for OnPricingUpdate to be called.
	select {
	case <-p.Done:
		return false
	case <-time.After(p.timeout):
		return true
	}
}
