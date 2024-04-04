package mvc

import (
	"context"

	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/proto/types"
)

// IngestUsecase represent the ingest's usecases
type IngestUsecase interface {
	// ProcessBlockData processes the block data as defined by height, takerFeesMap and poolData
	// Prior to loading pools into the repository, the pools are transformed and instrumented with pool TVL data.
	ProcessBlockData(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap, poolData []*types.PoolData) (err error)
}
