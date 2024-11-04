package mocks

import (
	"context"
	"errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"google.golang.org/grpc"
)

type PassthroughGRPCClientMock struct {
	MockAllBalancesCb                   func(ctx context.Context, address string) (sdk.Coins, error)
	MockAccountLockedCoinsCb            func(ctx context.Context, address string) (sdk.Coins, error)
	MockAccountUnlockingCoinsCb         func(ctx context.Context, address string) (sdk.Coins, error)
	MockDelegatorDelegationsCb          func(ctx context.Context, address string) (sdk.Coins, error)
	MockDelegatorUnbondingDelegationsCb func(ctx context.Context, address string) (sdk.Coins, error)
	MockUserPositionsBalancesCb         func(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error)
	MockDelegationRewardsCb             func(ctx context.Context, address string) (sdk.Coins, error)
}

// GetChainGRPCClient implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) GetChainGRPCClient() grpc.ClientConnInterface {
	panic("unimplemented")
}

// AccountLockedCoins implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) AccountLockedCoins(ctx context.Context, address string) (sdk.Coins, error) {
	if p.MockAccountLockedCoinsCb != nil {
		return p.MockAccountLockedCoinsCb(ctx, address)
	}

	return nil, errors.New("MockAccountLockedCoinsCb is not implemented")
}

// AllBalances implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) AllBalances(ctx context.Context, address string) (sdk.Coins, error) {
	if p.MockAllBalancesCb != nil {
		return p.MockAllBalancesCb(ctx, address)
	}

	return nil, errors.New("MockAllBalancesCb is not implemented")
}

// DelegatorDelegations implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) DelegatorDelegations(ctx context.Context, address string) (sdk.Coins, error) {
	if p.MockDelegatorDelegationsCb != nil {
		return p.MockDelegatorDelegationsCb(ctx, address)
	}

	return nil, errors.New("MockDelegatorDelegationsCb is not implemented")
}

// DelegatorUnbondingDelegations implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) DelegatorUnbondingDelegations(ctx context.Context, address string) (sdk.Coins, error) {
	if p.MockDelegatorUnbondingDelegationsCb != nil {
		return p.MockDelegatorUnbondingDelegationsCb(ctx, address)
	}

	return nil, errors.New("MockDelegatorUnbondingDelegationsCb is not implemented")
}

// UserPositionsBalances implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) UserPositionsBalances(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error) {
	if p.MockUserPositionsBalancesCb != nil {
		return p.MockUserPositionsBalancesCb(ctx, address)
	}

	return nil, nil, errors.New("MockUserPositionsBalancesCb is not implemented")
}

// AccountUnlockingCoins implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) AccountUnlockingCoins(ctx context.Context, address string) (sdk.Coins, error) {
	if p.MockAccountUnlockingCoinsCb != nil {
		return p.MockAccountUnlockingCoinsCb(ctx, address)
	}

	return nil, errors.New("MockAccountLockedCoinsCb is not implemented")
}

// DelegationRewards implements passthroughdomain.PassthroughGRPCClient.
func (p *PassthroughGRPCClientMock) DelegationRewards(ctx context.Context, address string) (sdk.Coins, error) {
	if p.MockDelegationRewardsCb != nil {
		return p.MockDelegationRewardsCb(ctx, address)
	}

	return nil, errors.New("MockDelegationRewardsCb is not implemented")
}

var _ passthroughdomain.PassthroughGRPCClient = &PassthroughGRPCClientMock{}
