package types_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"

	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/ingest/types"
)

// Validates that concurrent updates of the liquidity cap and of its error are both kept.
// Each setter reads the model, changes one field and stores it back, so without
// serialisation one update overwrites the other's field with the value it read earlier.
func TestPoolWrapper_ConcurrentLiquidityCapUpdates(t *testing.T) {
	const iterations = 2000

	for i := range iterations {
		pool := types.NewPool(&mocks.ChainPoolMock{ID: 1}, types.SQSPool{})

		var wg sync.WaitGroup
		wg.Go(func() { pool.SetLiquidityCap(osmomath.NewInt(int64(i + 1))) })
		wg.Go(func() { pool.SetLiquidityCapError("error") })
		wg.Wait()

		require.Equal(t, osmomath.NewInt(int64(i+1)).String(), pool.GetLiquidityCap().String())
		require.Equal(t, "error", pool.GetLiquidityCapError())
	}
}
