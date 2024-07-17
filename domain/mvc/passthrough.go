package mvc

import (
	"context"

	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
)

// PassthroughUsecase represents the passthrough module's use cases
type PassthroughUsecase interface {
	GetPortfolioAssets(ctx context.Context, address string) (passthroughdomain.PortfolioAssetsResult, error)
}

type PassthroughTokensUseCase interface{}
