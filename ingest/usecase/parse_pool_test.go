package usecase_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/v31/app"
	osmojson "github.com/osmosis-labs/osmosis/v31/ingest/types/json"
	prototypes "github.com/osmosis-labs/osmosis/v31/ingest/types/proto/types"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/ingest/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"
)

// TestParsePool_NodeEncodedModels feeds parsePool the payload the Osmosis node sends: the chain
// model as protobuf JSON from the interface registry codec, and the SQS and tick models encoded
// with the Osmosis jsoniter wrapper. SQS decodes the latter with encoding/json/v2; the result
// must match what the wrapper itself decodes.
func TestParsePool_NodeEncodedModels(t *testing.T) {
	pools, tickModels, err := parsing.ReadPools("../../router/usecase/routertesting/parsing/pools.json")
	require.NoError(t, err)
	require.NotEmpty(t, pools)

	cdc := app.MakeEncodingConfig().Marshaler

	for _, pool := range pools {
		chainModel, err := cdc.MarshalInterfaceJSON(pool.GetUnderlyingPool())
		require.NoError(t, err)
		sqsModel, err := osmojson.Marshal(pool.GetSQSPoolModel())
		require.NoError(t, err)
		poolData := &prototypes.PoolData{ChainModel: chainModel, SqsModel: sqsModel}

		tickModel := tickModels[pool.GetId()]
		if pool.GetType() == poolmanagertypes.Concentrated {
			require.NotNil(t, tickModel)
			poolData.TickModel, err = osmojson.Marshal(tickModel)
			require.NoError(t, err)
		}

		parsed, err := usecase.ParsePool(cdc, poolData)
		require.NoError(t, err, "pool %d", pool.GetId())
		require.Equal(t, pool.GetId(), parsed.GetId())
		require.Equal(t, pool.GetType(), parsed.GetType())

		// parsePool post-processes the SQS model, so compare with the same processing applied
		// to the wrapper's decoding of the same bytes.
		var wantSQSModel ingesttypes.SQSPool
		require.NoError(t, osmojson.Unmarshal(sqsModel, &wantSQSModel))
		require.NoError(t, usecase.ProcessSQSModelMut(&wantSQSModel))
		requireSameWrapperEncoding(t, wantSQSModel, parsed.GetSQSPoolModel())

		if pool.GetType() == poolmanagertypes.Concentrated {
			gotTickModel, err := parsed.GetTickModel()
			require.NoError(t, err)
			requireSameWrapperEncoding(t, tickModel, gotTickModel)
		}
	}
}

func requireSameWrapperEncoding(t *testing.T, want, got any) {
	t.Helper()
	wantBz, err := osmojson.Marshal(want)
	require.NoError(t, err)
	gotBz, err := osmojson.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, string(wantBz), string(gotBz))
}
