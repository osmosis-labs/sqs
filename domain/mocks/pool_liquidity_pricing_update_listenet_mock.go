package mocks

import (
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

type PoolLiquidityPricingMock struct {
	// represents the last height this mock was called with.
	lastHeightCalled int64
}

var _ domain.PoolLiquidityComputeListener = &PoolLiquidityPricingMock{}

// OnPoolLiquidityCompute implements domain.PoolLiquidityComputeListener.
func (p *PoolLiquidityPricingMock) OnPoolLiquidityCompute(height int64) error {
	p.lastHeightCalled = height
	return nil
}

// GetLastHeightCalled returns the last heigh when this mock was executed.
func (p *PoolLiquidityPricingMock) GetLastHeightCalled() int64 {
	return p.lastHeightCalled
}

func NewPoolLiquidityPricingMock(timeout time.Duration) *PoolLiquidityPricingMock {
	return &PoolLiquidityPricingMock{}
}
