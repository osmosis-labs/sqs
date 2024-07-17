package mocks

import (
	"context"

	"github.com/cosmos/cosmos-sdk/types"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
)

type PassthroughGRPCClientMock struct {
	MockAllBalancesCb                   func(ctx context.Context, address string) (types.Coins, error)
	MockAccountLockedCoinsCb            func(ctx context.Context, address string) (types.Coins, error)
	MockDelegatorDelegationsCb          func(ctx context.Context, address string) (types.Coins, error)
	MockDelegatorUnbondingDelegationsCb func(ctx context.Context, address string) (types.Coins, error)
	MockUserPositionsBalancesCb         func(ctx context.Context, address string) (types.Coins, error)
}

// AccountLockedCoins implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) AccountLockedCoins(ctx context.Context, address string) (types.Coins, error) {
	if p.MockAccountLockedCoinsCb != nil {
		return p.MockAccountLockedCoinsCb(ctx, address)
	}

	return nil, errors.New("MockAccountLockedCoinsCb is not implemented")
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
	if p.MockDelegatorDelegationsCb != nil {
		return p.MockDelegatorDelegationsCb(ctx, address)
	}

	panic("unimplemented")
}

	if p.MockDelegatorUnbondingDelegationsCb != nil {
		return p.MockDelegatorUnbondingDelegationsCb(ctx, address)
	}

	return nil, errors.New("MockDelegatorUnbondingDelegationsCb is not implemented")

// UserPositionsBalances implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) UserPositionsBalances(ctx context.Context, address string) (types.Coins, error) {
	if p.MockUserPositionsBalancesCb != nil {
		return p.MockUserPositionsBalancesCb(ctx, address)
	}

	return nil, errors.New("MockUserPositionsBalancesCb is not implemented")
}

var _ passthroughdomain.PassthroughGRPCClient = &PassthroughGRPCClientMock{}
