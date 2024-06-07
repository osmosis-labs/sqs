package mvc

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// PassthroughUsecase represents the passthrough module's use cases
type PassthroughUsecase interface {
	GetAccountCoinsTotal(ctx context.Context, address string) (sdk.Coins, error)
}
