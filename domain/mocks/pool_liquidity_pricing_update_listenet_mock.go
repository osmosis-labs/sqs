package mocks

import (
	"context"
	"time"

	"github.com/osmosis-labs/sqs/domain"
)

type PoolLiquidityPricingMock struct {
	// represents the last height this mock was called with.
	lastHeightCalled uint64
}

var _ domain.PoolLiquidityComputeListener = &PoolLiquidityPricingMock{}

// OnPoolLiquidityCompute implements domain.PoolLiquidityComputeListener.
func (p *PoolLiquidityPricingMock) OnPoolLiquidityCompute(ctx context.Context, height uint64, blockPoolMetaData domain.BlockPoolMetadata) error {
	p.lastHeightCalled = height
	return nil
}

// GetLastHeightCalled returns the last heigh when this mock was executed.
func (p *PoolLiquidityPricingMock) GetLastHeightCalled() uint64 {
	return p.lastHeightCalled
}

func NewPoolLiquidityPricingMock(timeout time.Duration) *PoolLiquidityPricingMock {
	return &PoolLiquidityPricingMock{}
}
