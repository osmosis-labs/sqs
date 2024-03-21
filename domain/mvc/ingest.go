package mvc

import (
	"context"

	"github.com/osmosis-labs/sqs/sqsdomain"
	prototypes "github.com/osmosis-labs/sqs/sqsdomain/proto/types"
)

// IngestUsecase represent the ingest's usecases
type IngestUsecase interface {
	// ProcessPoolChunk processes the pool data chunk, returning error if any.
	// Caches the given pools in-memory until the end of the block processing.
	ProcessPoolChunk(ctx context.Context, poolChunk []*prototypes.PoolData) error

	// StartBlockProcess signifies the start of the given block height processing
	// It persists the given taker fee into the repository.
	StartBlockProcess(ctx context.Context, height uint64, takerFeesMap sqsdomain.TakerFeeMap) (err error)

	// EndBlockProcessing ends the given block processing on success, storing the height
	// internally.
	// Persists the given height as well as any previously processed pools in-store.
	// Resets the internal pools cache to be empty.
	EndBlockProcess(ctx context.Context, height uint64) (err error)
}
