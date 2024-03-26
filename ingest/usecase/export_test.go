package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
)

type (
	IngestUseCaseImpl = ingestUseCase
)

func (p *ingestUseCase) ComputeCoinTVL(ctx context.Context, coin sdk.Coin) (osmomath.Dec, error) {
	return p.computeCoinTVL(ctx, coin)
}
