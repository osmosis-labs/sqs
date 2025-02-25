package orderbookorderscache_test

import (
	"fmt"
	"testing"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookorderscache "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/orderscache"
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
