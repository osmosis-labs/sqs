package mocks

import (
	"context"
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type PricingUpdateListenerMock struct {
	Height                 int
	PricesBaseQuteDenomMap map[string]map[string]osmomath.BigDec
	QuoteDenom             string

	Done chan struct{}

	MockErrorToReturn error

	timeout time.Duration
}

func NewPricingListenerMock(timeout time.Duration) *PricingUpdateListenerMock {
	return &PricingUpdateListenerMock{
		Done:                   make(chan struct{}),
		PricesBaseQuteDenomMap: make(map[string]map[string]osmomath.BigDec),
		timeout:                timeout,
	}
}

var _ domain.PricingUpdateListener = &PricingUpdateListenerMock{}

// OnPricingUpdate implements worker.PricingUpdateListener.
func (p *PricingUpdateListenerMock) OnPricingUpdate(ctx context.Context, height int64, blockMetadata domain.BlockPoolMetadata, pricesBaseQuoteDenomMap domain.PricesResult, quoteDenom string) error {
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
