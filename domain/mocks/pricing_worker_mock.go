package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
)

var _ domain.PricingWorker = &PricingWorkerMock{}

// PricingWorkerMock is a mock implementation of the PricingWorker interface
type PricingWorkerMock struct {
	UpdatePricesAsyncFunc func(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata)
	UpdatePricesSyncFunc  func(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata)
	RegisterListenerFunc  func(listener domain.PricingUpdateListener)
}

func (m *PricingWorkerMock) UpdatePricesAsync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	if m.UpdatePricesAsyncFunc != nil {
		m.UpdatePricesAsyncFunc(ctx, height, uniqueBlockPoolMetaData)
	}
}

func (m *PricingWorkerMock) UpdatePricesSync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	if m.UpdatePricesSyncFunc != nil {
		m.UpdatePricesSyncFunc(ctx, height, uniqueBlockPoolMetaData)
	}
}

func (m *PricingWorkerMock) RegisterListener(listener domain.PricingUpdateListener) {
	if m.RegisterListenerFunc != nil {
		m.RegisterListenerFunc(listener)
	}
}
