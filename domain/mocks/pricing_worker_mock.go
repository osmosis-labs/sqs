package mocks

import "github.com/osmosis-labs/sqs/domain"

var _ domain.PricingWorker = &PricingWorkerMock{}

// PricingWorkerMock is a mock implementation of the PricingWorker interface
type PricingWorkerMock struct {
	UpdatePricesAsyncFunc func(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata)
	UpdatePricesSyncFunc  func(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata)
	RegisterListenerFunc  func(listener domain.PricingUpdateListener)
}

func (m *PricingWorkerMock) UpdatePricesAsync(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	if m.UpdatePricesAsyncFunc != nil {
		m.UpdatePricesAsyncFunc(height, uniqueBlockPoolMetaData)
	}
}

func (m *PricingWorkerMock) UpdatePricesSync(height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) {
	if m.UpdatePricesSyncFunc != nil {
		m.UpdatePricesSyncFunc(height, uniqueBlockPoolMetaData)
	}
}

func (m *PricingWorkerMock) RegisterListener(listener domain.PricingUpdateListener) {
	if m.RegisterListenerFunc != nil {
		m.RegisterListenerFunc(listener)
	}
}
