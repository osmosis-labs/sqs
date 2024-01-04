package mvc

import (
	"context"
)

type ChainInfoUsecase interface {
	GetLatestHeight(ctx context.Context) (uint64, error)
}
