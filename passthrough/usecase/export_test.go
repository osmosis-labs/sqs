package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	GammSharePrefix     = gammSharePrefix
	DenomShareSeparator = denomShareSeparator
)

func (p *passthroughUseCase) GetBankBalances(ctx context.Context, address string) (sdk.Coins, error) {
	return p.getBankBalances(ctx, address)
}

func (p *passthroughUseCase) HandleGammShares(balance sdk.Coin) (sdk.Coins, error) {
	return p.handleGammShares(balance)
}
