package claimbot_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"

	"github.com/stretchr/testify/assert"
)

func TestGetOrderbooks(t *testing.T) {
	tests := []struct {
		name        string
		blockHeight uint64
		metadata    domain.BlockPoolMetadata
		setupMocks  func(*mocks.PoolsUsecaseMock)
		want        []domain.CanonicalOrderBooksResult
		err         bool
	}{
		{
			name:        "Metadata contains all canonical orderbooks but one",
			blockHeight: 1000,
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}, 2: {}, 3: {}},
			},
			setupMocks: func(poolsUsecase *mocks.PoolsUsecaseMock) {
				poolsUsecase.WithGetAllCanonicalOrderbookPoolIDs([]domain.CanonicalOrderBooksResult{
					{PoolID: 1}, {PoolID: 2}, {PoolID: 3}, {PoolID: 4},
				}, nil)
			},
			want: []domain.CanonicalOrderBooksResult{
				{PoolID: 1}, {PoolID: 2}, {PoolID: 3},
			},
			err: false,
		},
		{
			name:        "Metadata contains only canonical orderbooks",
			blockHeight: 1893,
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}, 2: {}, 3: {}},
			},
			setupMocks: func(poolsUsecase *mocks.PoolsUsecaseMock) {
				poolsUsecase.WithGetAllCanonicalOrderbookPoolIDs([]domain.CanonicalOrderBooksResult{
					{PoolID: 1}, {PoolID: 2}, {PoolID: 3},
				}, nil)
			},
			want: []domain.CanonicalOrderBooksResult{
				{PoolID: 1}, {PoolID: 2}, {PoolID: 3},
			},
			err: false,
		},
		{
			name:        "Error getting all canonical orderbook pool IDs",
			blockHeight: 2000,
			metadata:    domain.BlockPoolMetadata{},
			setupMocks: func(poolsUsecase *mocks.PoolsUsecaseMock) {
				poolsUsecase.WithGetAllCanonicalOrderbookPoolIDs(nil, assert.AnError)
			},
			want: nil,
			err:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			poolsUsecase := mocks.PoolsUsecaseMock{}

			tt.setupMocks(&poolsUsecase)

			got, err := claimbot.GetOrderbooks(&poolsUsecase, tt.blockHeight, tt.metadata)
			if tt.err {
				assert.Error(t, err)
				return
			}

			assert.Equal(t, got, tt.want)
		})
	}
}
