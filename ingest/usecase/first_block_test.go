package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/v31/app"
	osmoingesttypes "github.com/osmosis-labs/osmosis/v31/ingest/types"
	prototypes "github.com/osmosis-labs/osmosis/v31/ingest/types/proto/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/log"
)

// Validates that a first block that fails before its completion is signalled does not leave
// every later block waiting for it: the next block carrying all pools is processed as the
// first block, and regular blocks follow.
func TestProcessBlockData_FirstBlockFailureDoesNotBlockLaterBlocks(t *testing.T) {
	const (
		// Above the pool count that marks a block as the first one after start-up.
		firstBlockPoolCount = 500

		processTimeout = 30 * time.Second
	)

	var (
		errStorePools = errors.New("store pools failed")
		failStore     = true
	)

	poolsUsecase := &mocks.PoolsUsecaseMock{
		StorePoolsFunc: func([]ingesttypes.PoolI) error {
			if failStore {
				return errStorePools
			}
			return nil
		},
		GetAllOrderbookPoolIDsFunc: func() ([]domain.CanonicalOrderBooksResult, error) {
			return nil, nil
		},
	}

	ingester, err := usecase.NewIngestUsecase(
		poolsUsecase,
		&mocks.RouterUsecaseMock{},
		&mocks.RouterUsecaseMock{},
		&mocks.TokensUsecaseMock{},
		&mocks.ChainInfoUsecaseMock{},
		app.MakeEncodingConfig().Marshaler,
		&mocks.PricingWorkerMock{},
		&mocks.CandidateRouteSearchDataWorkerMock{},
		nil,
		&log.NoOpLogger{},
	)
	require.NoError(t, err)

	// Pool data that fails to parse is skipped, so empty entries make a block of the first
	// block's size without any real pool.
	firstBlockPools := make([]*prototypes.PoolData, firstBlockPoolCount)
	for i := range firstBlockPools {
		firstBlockPools[i] = &prototypes.PoolData{}
	}

	process := func(height uint64, pools []*prototypes.PoolData) error {
		result := make(chan error, 1)
		go func() {
			result <- ingester.ProcessBlockData(context.Background(), height, osmoingesttypes.TakerFeeMap{}, pools)
		}()

		select {
		case err := <-result:
			return err
		case <-time.After(processTimeout):
			t.Fatalf("block %d did not finish processing: it is waiting for a first block that never completed", height)
			return nil
		}
	}

	// The first block fails before reaching the point where its completion is signalled.
	require.ErrorIs(t, process(10, firstBlockPools), errStorePools)

	// The node re-sends all pools on the next block, which must be processed.
	failStore = false
	require.NoError(t, process(11, firstBlockPools))

	// A regular block after it must not wait either.
	require.NoError(t, process(12, []*prototypes.PoolData{{}}))
}
