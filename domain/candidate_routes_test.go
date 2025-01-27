package domain_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/stretchr/testify/require"
)

// This test validates the ShouldSkipPool() method of the candidate route search
// options by registering two filters:
// 1. PoolID filter
// 2. Orderbook filter.
//
// It then validates that if at least one of the filters is matched, ShouldSkipPool()
// would return true
func TestCandidateRouteSearchOptions_ShouldSkipPool(t *testing.T) {

	const (
		defaultPoolID = uint64(1)
	)

	var (
		defaultNonOrderBookPool = ingesttypes.PoolWrapper{
			ChainModel: &mocks.ChainPoolMock{
				ID: defaultPoolID,
			},
		}

		// instruments the given pool with orderbook data, returning new copy.
		withOrderBookPool = func(pool ingesttypes.PoolWrapper) ingesttypes.PoolWrapper {
			pool.SQSModel = ingesttypes.SQSPool{
				CosmWasmPoolModel: &cosmwasmpool.CosmWasmPoolModel{
					ContractInfo: cosmwasmpool.ContractInfo{
						Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME,
						Version:  cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
					},
					Data: cosmwasmpool.CosmWasmPoolData{

						Orderbook: &cosmwasmpool.OrderbookData{},
					},
				},
			}
			return pool
		}

		// instruments the given pool with a new id returning new copy
		withPoolID = func(pool ingesttypes.PoolWrapper, newPoolID uint64) ingesttypes.PoolWrapper {
			pool.ChainModel = &mocks.ChainPoolMock{
				ID: newPoolID,
			}
			return pool
		}

		defaultOrderbookPool = withPoolID(withOrderBookPool(defaultNonOrderBookPool), defaultPoolID+1)
	)

	tests := []struct {
		name string

		poolIDsToFilter map[uint64]struct{}

		poolToTest ingesttypes.PoolWrapper

		expectedShouldSkip bool
	}{
		{
			name: "non orderbook pool, not filtered -> returns false",

			poolToTest: defaultNonOrderBookPool,

			expectedShouldSkip: false,
		},

		{
			name: "non orderbook pool, filtered by id -> returns true",

			poolToTest: defaultNonOrderBookPool,

			poolIDsToFilter: map[uint64]struct{}{
				defaultPoolID: {},
			},

			expectedShouldSkip: true,
		},

		{
			name: "order book pool -> returns true",

			poolToTest: defaultOrderbookPool,

			expectedShouldSkip: true,
		},
		{
			name: "both filters match -> return true",

			poolToTest: defaultOrderbookPool,

			poolIDsToFilter: map[uint64]struct{}{
				defaultOrderbookPool.GetId(): {},
			},

			expectedShouldSkip: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {

			// Set up pool ID filter
			poolIDFilter := domain.CandidateRoutePoolIDFilterOptionCb{
				PoolIDsToSkip: tc.poolIDsToFilter,
			}

			// Initialize options with 2 filters:
			// 1. By pool ID
			// 2. All order books.
			opts := domain.CandidateRouteSearchOptions{
				PoolFiltersAnyOf: []domain.CandidateRoutePoolFiltrerCb{
					poolIDFilter.ShouldSkipPool,
					domain.ShouldSkipOrderbookPool,
				},
			}

			// System under test.
			shouldSkip := opts.ShouldSkipPool(&tc.poolToTest)

			// Validate result.
			require.Equal(t, tc.expectedShouldSkip, shouldSkip)
		})
	}
}
