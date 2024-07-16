package mvc

import (
	"context"

	"github.com/osmosis-labs/sqs/domain/passthrough"
)

// PassthroughUsecase represents the passthrough module's use cases
type PassthroughUsecase interface {
	GetAccountCoinsTotal(ctx context.Context, address string) ([]passthroughdomain.AccountCoinsResult, error)
}
