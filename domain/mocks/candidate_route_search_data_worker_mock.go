package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
)

// CandidateRouteSearchDataWorkerMock is a mock for CandidateRouteSearchDataWorker.
type CandidateRouteSearchDataWorkerMock struct {
	ComputeSearchDataSyncFunc  func(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) error
	ComputeSearchDataAsyncFunc func(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) error

	RegisterListenerFunc func(listener domain.CandidateRouteSearchDataUpdateListener)
}

func (m *CandidateRouteSearchDataWorkerMock) ComputeSearchDataSync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) error {
	if m.ComputeSearchDataSyncFunc != nil {
		return m.ComputeSearchDataSyncFunc(ctx, height, uniqueBlockPoolMetaData)
	}
	return nil
}

func (m *CandidateRouteSearchDataWorkerMock) ComputeSearchDataAsync(ctx context.Context, height uint64, uniqueBlockPoolMetaData domain.BlockPoolMetadata) error {
	if m.ComputeSearchDataAsyncFunc != nil {
		return m.ComputeSearchDataAsyncFunc(ctx, height, uniqueBlockPoolMetaData)
	}
	return nil
}

func (m *CandidateRouteSearchDataWorkerMock) RegisterListener(listener domain.CandidateRouteSearchDataUpdateListener) {
	if m.RegisterListenerFunc != nil {
		m.RegisterListenerFunc(listener)
	}
}
