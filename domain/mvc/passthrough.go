package mvc

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type PassthroughUsecase interface {
	GetBalances(ctx context.Context, address string) (sdk.Coins, error)
}
