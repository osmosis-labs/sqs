package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/passthrough/clients"
)

type passthroughUsecase struct {
	bankClient clients.BankClientI
}

var _ mvc.PassthroughUsecase = &passthroughUsecase{}

// NewPassthroughUsecase will create a new passthrough use case object
func NewPassthroughUsecase(bankClient clients.BankClientI) mvc.PassthroughUsecase {
	return &passthroughUsecase{
		bankClient: bankClient,
	}
}

// GetBalances returns all balances for a given address.
func (p *passthroughUsecase) GetBalances(ctx context.Context, address string) (sdk.Coins, error) {
	balances, err := p.bankClient.GetBalance(ctx, address)
	if err != nil {
		return nil, err
	}

	return balances, nil
}
