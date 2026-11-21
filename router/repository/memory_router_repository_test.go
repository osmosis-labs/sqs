package routerrepo_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	sqsatomic "github.com/osmosis-labs/sqs/domain/sync/atomic"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/stretchr/testify/require"
)

// Validates that back-filling a zero liquidity cap from the pool handler does not mutate
// the previously published candidate route data, which request goroutines may still be reading.
func TestSetCandidateRouteSearchData_DoesNotMutatePublishedData(t *testing.T) {
	const (
		denomA = "denomA"
		denomB = "denomB"
		poolID = 1
	)

	repository := routerrepo.New(&log.NoOpLogger{})
	repository.SetPoolHandler(&mocks.PoolHandlerMock{
		Pools: []ingesttypes.PoolI{&mocks.MockRoutablePool{ID: poolID, PoolLiquidityCap: osmomath.NewInt(100)}},
	})

	repository.SetCandidateRouteSearchData(map[string]*domain.CandidateRouteDenomData{
		denomA: {SortedPools: []domain.CandidatePoolWrapper{{ID: poolID, PoolLiquidityCap: sqsatomic.NewUint64(0)}}},
	})

	published, err := repository.GetDenomData(denomA)
	require.NoError(t, err)

	// System under test: the next block updates a different denom, back-filling denomA's zero cap.
	repository.SetCandidateRouteSearchData(map[string]*domain.CandidateRouteDenomData{
		denomB: {SortedPools: []domain.CandidatePoolWrapper{{ID: poolID + 1, PoolLiquidityCap: sqsatomic.NewUint64(5)}}},
	})

	updated, err := repository.GetDenomData(denomA)
	require.NoError(t, err)
	require.Equal(t, uint64(100), updated.SortedPools[0].GetPoolLiquidityCap())

	// The previously published entry is unchanged.
	require.Equal(t, uint64(0), published.SortedPools[0].GetPoolLiquidityCap())
}
