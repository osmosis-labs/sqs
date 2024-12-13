package mvc

import (
	"context"

	ingesttypes "github.com/osmosis-labs/osmosis/v28/ingest/types"
	"github.com/osmosis-labs/osmosis/v28/ingest/types/proto/types"
	"github.com/osmosis-labs/sqs/domain"
)

// IngestUsecase represent the ingest's usecases
type IngestUsecase interface {
	// ProcessBlockData processes the block data as defined by height, takerFeesMap and poolData
	// Prior to loading pools into the repository, the pools are transformed and instrumented with pool TVL data.
	ProcessBlockData(ctx context.Context, height uint64, takerFeesMap ingesttypes.TakerFeeMap, poolData []*types.PoolData) (err error)

	// RegisterEndBlockProcessPlugin registers the end block process plugin
	// That is called at the end of the block
	RegisterEndBlockProcessPlugin(plugin domain.EndBlockProcessPlugin)
}
