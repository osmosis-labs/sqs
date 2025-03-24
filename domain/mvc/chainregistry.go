package mvc

import (
	"context"

	api "github.com/osmosis-labs/sqs/pkg/api/v1beta1/chainregistry"
)

// ChainregistryUsecase represents the chainregistry module's use cases
type ChainregistryUsecase interface {
	GetFeeTokens(ctx context.Context) ([]*api.FeeToken, error)
}
