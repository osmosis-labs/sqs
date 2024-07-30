package mvc

import (
	"context"

	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
)

// PassthroughUsecase represents the passthrough module's use cases
type PassthroughUsecase interface {
	// GetPortfolioAssets returns the total value of the assets in the portfolio
	// of the user with the given address.
	GetPortfolioAssets(ctx context.Context, address string) (passthroughdomain.PortfolioAssetsResult2, error)
}
