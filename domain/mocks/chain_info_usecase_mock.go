package mocks

import (
	"context"

	chaininfoclient "github.com/osmosis-labs/sqs/chaininfo/client"
	"github.com/osmosis-labs/sqs/domain/mvc"

	ctypes "github.com/cometbft/cometbft/rpc/core/types"
)

var (
	_ mvc.ChainInfoUsecase   = &ChainInfoUsecaseMock{}
	_ chaininfoclient.Client = &ChainInfoClientMock{}
)

type ChainInfoClientMock struct {
	GetLatestHeightFunc func(ctx context.Context) (uint64, error)
	GetStatusFunc       func(ctx context.Context) (*ctypes.ResultStatus, error)
}

func (m *ChainInfoClientMock) GetLatestHeight(ctx context.Context) (uint64, error) {
	if m.GetLatestHeightFunc != nil {
		return m.GetLatestHeightFunc(ctx)
	}
	return 0, nil
}

func (m *ChainInfoClientMock) GetStatus(ctx context.Context) (*ctypes.ResultStatus, error) {
	if m.GetStatusFunc != nil {
		return m.GetStatusFunc(ctx)
	}
	return nil, nil // nolint:nilnil
}

// ChainInfoUsecaseMock is a mock implementation of the ChainInfoUsecase interface
type ChainInfoUsecaseMock struct {
	GetLatestHeightFunc                         func() (uint64, error)
	StoreLatestHeightFunc                       func(height uint64)
	ValidatePriceUpdatesFunc                    func() error
	ValidatePoolLiquidityUpdatesFunc            func() error
	ValidateCandidateRouteSearchDataUpdatesFunc func() error
	GetClientFunc                               func() chaininfoclient.Client
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

func (m *ChainInfoUsecaseMock) ValidatePoolLiquidityUpdates() error {
	if m.ValidatePoolLiquidityUpdatesFunc != nil {
		return m.ValidatePoolLiquidityUpdatesFunc()
	}
	return nil
}

func (m *ChainInfoUsecaseMock) ValidateCandidateRouteSearchDataUpdates() error {
	if m.ValidateCandidateRouteSearchDataUpdatesFunc != nil {
		return m.ValidateCandidateRouteSearchDataUpdatesFunc()
	}
	return nil
}

func (m *ChainInfoUsecaseMock) GetClient() chaininfoclient.Client {
	if m.GetClientFunc != nil {
		return m.GetClientFunc()
	}
	return nil
}
