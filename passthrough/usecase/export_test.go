package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
)

var (
	GammSharePrefix         = gammSharePrefix
	ConcentratedSharePrefix = concentratedSharePrefix
	DenomShareSeparator     = denomShareSeparator
)

func (p *passthroughUseCase) GetLockedCoins(ctx context.Context, address string) (sdk.Coins, error) {
	return p.getLockedCoins(ctx, address)
}

func (p *passthroughUseCase) GetBankBalances(ctx context.Context, address string) (sdk.Coins, error) {
	return p.getBankBalances(ctx, address)
}

func (p *passthroughUseCase) HandleGammShares(balance sdk.Coin) (sdk.Coins, error) {
	return p.handleGammShares(balance)
}

func (p *passthroughUseCase) ComputeCapitalizationForCoins(ctx context.Context, coins sdk.Coins) ([]passthroughdomain.AccountCoinsResult, osmomath.Dec, error) {
	return p.computeCapitalizationForCoins(ctx, coins)
}
