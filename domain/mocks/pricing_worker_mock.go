package mocks

import "github.com/osmosis-labs/sqs/domain"

var _ domain.PricingWorker = &PricingWorkerMock{}

// PricingWorkerMock is a mock implementation of the PricingWorker interface
type PricingWorkerMock struct {
	UpdatePricesAsyncFunc func(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata)
	RegisterListenerFunc  func(listener domain.PricingUpdateListener)
}

func (m *PricingWorkerMock) UpdatePricesAsync(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	if m.UpdatePricesAsyncFunc != nil {
		m.UpdatePricesAsyncFunc(height, uniqueBlockPoolMetaData)
	}
}

func (m *PricingWorkerMock) RegisterListener(listener domain.PricingUpdateListener) {
	if m.RegisterListenerFunc != nil {
		m.RegisterListenerFunc(listener)
	}
}
