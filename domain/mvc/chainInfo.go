package mvc

import (
	"context"

	"github.com/osmosis-labs/sqsdomain/repository"
)

// ChainInfoRepository represents the contract for a repository handling chain information
type ChainInfoRepository interface {
	// StoreLatestHeight stores the latest blockchain height
	StoreLatestHeight(ctx context.Context, tx repository.Tx, height uint64) error

	// GetLatestHeight retrieves the latest blockchain height
	GetLatestHeight(ctx context.Context) (uint64, error)
}

type ChainInfoUsecase interface {
	GetLatestHeight(ctx context.Context) (uint64, error)
}
