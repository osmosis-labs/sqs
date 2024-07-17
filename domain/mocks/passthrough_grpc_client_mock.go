package mocks

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
)

type PassthroughGRPCClientMock struct {
	MockAllBalancesCb func(ctx context.Context, address string) (types.Coins, error)
}

// AccountLockedCoins implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) AccountLockedCoins(ctx context.Context, address string) (types.Coins, error) {
	panic("unimplemented")
}

// AllBalances implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) AllBalances(ctx context.Context, address string) (types.Coins, error) {
	if p.MockAllBalancesCb != nil {
		return p.MockAllBalancesCb(ctx, address)
	}

	panic("unimplemented")
}

// DelegatorDelegations implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) DelegatorDelegations(ctx context.Context, address string) (types.Coins, error) {
	panic("unimplemented")
}

// DelegatorUnbondingDelegations implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) DelegatorUnbondingDelegations(ctx context.Context, address string) (types.Coins, error) {
	panic("unimplemented")
}

// UserPositionsBalances implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) UserPositionsBalances(ctx context.Context, address string) (types.Coins, error) {
	panic("unimplemented")
}

var _ passthroughdomain.PassthroughGRPCClient = &PassthroughGRPCClientMock{}
