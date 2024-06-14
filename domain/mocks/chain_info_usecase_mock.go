package mocks

import "github.com/osmosis-labs/sqs/domain/mvc"

var _ mvc.ChainInfoUsecase = &ChainInfoUsecaseMock{}

// ChainInfoUsecaseMock is a mock implementation of the ChainInfoUsecase interface
type ChainInfoUsecaseMock struct {
	GetLatestHeightFunc      func() (uint64, error)
	StoreLatestHeightFunc    func(height uint64)
	ValidatePriceUpdatesFunc func() error
}

func (m *ChainInfoUsecaseMock) GetLatestHeight() (uint64, error) {
	if m.GetLatestHeightFunc != nil {
		return m.GetLatestHeightFunc()
	}
	return 0, nil
}

func (m *ChainInfoUsecaseMock) StoreLatestHeight(height uint64) {
	if m.StoreLatestHeightFunc != nil {
		m.StoreLatestHeightFunc(height)
	}
}

func (m *ChainInfoUsecaseMock) ValidatePriceUpdates() error {
	if m.ValidatePriceUpdatesFunc != nil {
		return m.ValidatePriceUpdatesFunc()
	}
	return nil
}
