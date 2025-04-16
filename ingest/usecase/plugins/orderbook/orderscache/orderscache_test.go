package orderbookorderscache_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookorderscache "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/orderscache"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"

	"github.com/stretchr/testify/suite"
)

// OrderbookTestHelper is a helper struct for the orderbook usecase tests
type OrdersCacheTestSuite struct {
	routertesting.RouterTestHelper
}

// SetupTest sets up the test suite
func TestOrderbookUsecaseTestSuite(t *testing.T) {
	suite.Run(t, new(OrdersCacheTestSuite))
}

func (s *OrdersCacheTestSuite) TestProcessEndBlock() {
	testCases := []struct {
		name          string
		setupMocks    func(*mocks.OrderbookRepositoryMock, *mocks.PoolsUsecaseMock, *mocks.PassthroughGRPCClientMock)
		metadata      domain.BlockPoolMetadata
		expectedError error
	}{
		{
			name: "Success - Process orderbooks and store orders",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, poolsUsecase *mocks.PoolsUsecaseMock, grpcClient *mocks.PassthroughGRPCClientMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{{PoolID: 1, ContractAddress: "addr1"}}, nil
				}
				grpcClient.MockGetOrderbookOrdersRawCb = func(ctx context.Context, poolID uint64) ([][]byte, error) {
					return [][]byte{[]byte(`{"order_id":1,"owner":"owner1","tick_index_taker_to_maker":100,"amount_taker":"100","amount_maker":"100"}`)}, nil
				}

				orderbookRepo.WithStoreOrders(nil)
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}},
			},
			expectedError: nil,
		},
		{
			name: "Error - Failed GetAllCanonicalOrderbookPoolIDsFunc call",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, poolsUsecase *mocks.PoolsUsecaseMock, grpcClient *mocks.PassthroughGRPCClientMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return nil, fmt.Errorf("failed to get orderbooks")
				}
			},
			metadata:      domain.BlockPoolMetadata{},
			expectedError: fmt.Errorf("failed to get orderbooks"),
		},
		{
			name: "Error - Failed GetOrderbookOrdersRaw call",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, poolsUsecase *mocks.PoolsUsecaseMock, grpcClient *mocks.PassthroughGRPCClientMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{{PoolID: 1, ContractAddress: "addr1"}}, nil
				}
				grpcClient.MockGetOrderbookOrdersRawCb = func(ctx context.Context, poolID uint64) ([][]byte, error) {
					return nil, fmt.Errorf("failed to get orders")
				}
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}},
			},
			expectedError: fmt.Errorf("failed to get orders"),
		},
		{
			name: "Error - Failed StoreOrders call",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, poolsUsecase *mocks.PoolsUsecaseMock, grpcClient *mocks.PassthroughGRPCClientMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{{PoolID: 1, ContractAddress: "addr1"}}, nil
				}
				grpcClient.MockGetOrderbookOrdersRawCb = func(ctx context.Context, poolID uint64) ([][]byte, error) {
					return nil, nil
				}

				orderbookRepo.WithStoreOrders(fmt.Errorf("failed to store orders"))
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}},
			},
			expectedError: fmt.Errorf("failed to store orders"),
		},
		{
			name: "Error - Invalid order JSON",
			setupMocks: func(orderbookRepo *mocks.OrderbookRepositoryMock, poolsUsecase *mocks.PoolsUsecaseMock, grpcClient *mocks.PassthroughGRPCClientMock) {
				poolsUsecase.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{{PoolID: 1, ContractAddress: "addr1"}}, nil
				}
				grpcClient.MockGetOrderbookOrdersRawCb = func(ctx context.Context, poolID uint64) ([][]byte, error) {
					return [][]byte{[]byte(`invalid json`)}, nil
				}
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}},
			},
			expectedError: fmt.Errorf("invalid character 'i' looking for beginning of value"),
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			orderbookRepo := &mocks.OrderbookRepositoryMock{}
			poolsUsecase := &mocks.PoolsUsecaseMock{}
			grpcClient := &mocks.PassthroughGRPCClientMock{}

			tc.setupMocks(orderbookRepo, poolsUsecase, grpcClient)

			cache := orderbookorderscache.New(orderbookRepo, poolsUsecase, &log.NoOpLogger{}, grpcClient)

			err := cache.ProcessEndBlock(context.Background(), 0, tc.metadata)
			if tc.expectedError != nil {
				s.Assert().Error(err)
				s.Assert().EqualError(err, tc.expectedError.Error())
			} else {
				s.Assert().NoError(err)
			}
		})
	}
}

func (s *OrdersCacheTestSuite) TestGetOrderbooks() {
	testCases := []struct {
		name           string
		setupMocks     func(*mocks.PoolsUsecaseMock)
		metadata       domain.BlockPoolMetadata
		expectedResult []domain.CanonicalOrderBooksResult
		expectedError  error
	}{
		{
			name: "Success - All orderbooks within metadata",
			setupMocks: func(mock *mocks.PoolsUsecaseMock) {
				mock.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{
						{PoolID: 1, ContractAddress: "addr1"},
						{PoolID: 2, ContractAddress: "addr2"},
					}, nil
				}
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}, 2: {}},
			},
			expectedResult: []domain.CanonicalOrderBooksResult{
				{PoolID: 1, ContractAddress: "addr1"},
				{PoolID: 2, ContractAddress: "addr2"},
			},
			expectedError: nil,
		},
		{
			name: "Success - Partial orderbooks within metadata",
			setupMocks: func(mock *mocks.PoolsUsecaseMock) {
				mock.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{
						{PoolID: 1, ContractAddress: "addr1"},
						{PoolID: 2, ContractAddress: "addr2"},
						{PoolID: 3, ContractAddress: "addr3"},
					}, nil
				}
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{1: {}, 3: {}},
			},
			expectedResult: []domain.CanonicalOrderBooksResult{
				{PoolID: 1, ContractAddress: "addr1"},
				{PoolID: 3, ContractAddress: "addr3"},
			},
			expectedError: nil,
		},
		{
			name: "Success - Empty result",
			setupMocks: func(mock *mocks.PoolsUsecaseMock) {
				mock.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return []domain.CanonicalOrderBooksResult{
						{PoolID: 1, ContractAddress: "addr1"},
						{PoolID: 2, ContractAddress: "addr2"},
					}, nil
				}
			},
			metadata: domain.BlockPoolMetadata{
				PoolIDs: map[uint64]struct{}{3: {}, 4: {}},
			},
			expectedResult: nil,
			expectedError:  nil,
		},
		{
			name: "Error - GetAllCanonicalOrderbookPoolIDs fails",
			setupMocks: func(mock *mocks.PoolsUsecaseMock) {
				mock.GetAllCanonicalOrderbookPoolIDsFunc = func() ([]domain.CanonicalOrderBooksResult, error) {
					return nil, fmt.Errorf("failed to get orderbooks")
				}
			},
			metadata:       domain.BlockPoolMetadata{},
			expectedResult: nil,
			expectedError:  fmt.Errorf("failed to get orderbooks"),
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			poolsUsecaseMock := &mocks.PoolsUsecaseMock{}

			tc.setupMocks(poolsUsecaseMock)

			result, err := orderbookorderscache.GetOrderbooks(poolsUsecaseMock, tc.metadata)
			if tc.expectedError != nil {
				s.Assert().Error(err)
				s.Assert().EqualError(err, tc.expectedError.Error())
			} else {
				s.Assert().NoError(err)
				s.Assert().Equal(tc.expectedResult, result)
			}
		})
	}
}
