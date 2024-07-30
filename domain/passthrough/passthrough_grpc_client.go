package passthroughdomain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	concentratedLiquidity "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/client/queryproto"
	lockup "github.com/osmosis-labs/osmosis/v25/x/lockup/types"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// PassthroughGRPCClient represents the GRPC client for the passthrough module to query the chain.
type PassthroughGRPCClient interface {
	// AccountLockedCoins returns the locked coins of the user with the given address.
	AccountLockedCoins(ctx context.Context, address string) (sdk.Coins, error)

	// AccountUnlockingCoins returns the unlocking coins of the user with the given address.
	AccountUnlockingCoins(ctx context.Context, address string) (sdk.Coins, error)

	// AllBalances returns all the balances of the user with the given address.
	AllBalances(ctx context.Context, address string) (sdk.Coins, error)

	// DelegatorDelegations returns the delegator delegations of the user with the given address.
	DelegatorDelegations(ctx context.Context, address string) (sdk.Coins, error)

	// DelegatorUnbondingDelegations returns the delegator unbonding delegations of the user with the given address.
	DelegatorUnbondingDelegations(ctx context.Context, address string) (sdk.Coins, error)

	// UserPositionsBalances returns the user concentrated positions balances of the user with the given address.
	// The first return is the pooled balance. The second return is the reward balance.
	UserPositionsBalances(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error)
}

type PassthroughFetchFn func(context.Context, string) (sdk.Coins, error)

type PassthroughFetchFunctionWithName struct {
	Name string
	Fn   PassthroughFetchFn
}

type passthroughGRPCClient struct {
	bankQueryClient                  banktypes.QueryClient
	stakingQueryClient               staking.QueryClient
	lockupQueryClient                lockup.QueryClient
	concentratedLiquidityQueryClient concentratedLiquidity.QueryClient
}

const (
	defaultBondDenom = "uosmo"
)

func NewPassthroughGRPCClient(grpcURI string) (PassthroughGRPCClient, error) {
	grpcClient, err := grpc.NewClient(grpcURI,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}

	return &passthroughGRPCClient{
		bankQueryClient:                  banktypes.NewQueryClient(grpcClient),
		stakingQueryClient:               staking.NewQueryClient(grpcClient),
		lockupQueryClient:                lockup.NewQueryClient(grpcClient),
		concentratedLiquidityQueryClient: concentratedLiquidity.NewQueryClient(grpcClient),
	}, nil
}

func (p *passthroughGRPCClient) AccountLockedCoins(ctx context.Context, address string) (sdk.Coins, error) {
	response, err := p.lockupQueryClient.AccountLockedCoins(ctx, &lockup.AccountLockedCoinsRequest{Owner: address})
	if err != nil {
		return nil, err
	}

	return response.Coins, nil
}

func (p *passthroughGRPCClient) AccountUnlockingCoins(ctx context.Context, address string) (sdk.Coins, error) {
	response, err := p.lockupQueryClient.AccountUnlockingCoins(ctx, &lockup.AccountUnlockingCoinsRequest{Owner: address})
	if err != nil {
		return nil, err
	}

	return response.Coins, nil
}

func (p *passthroughGRPCClient) AllBalances(ctx context.Context, address string) (sdk.Coins, error) {
	response, err := p.bankQueryClient.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{Address: address})
	if err != nil {
		return nil, err
	}

	return response.Balances, nil
}

func (p *passthroughGRPCClient) DelegatorDelegations(ctx context.Context, address string) (sdk.Coins, error) {
	response, err := p.stakingQueryClient.DelegatorDelegations(ctx, &staking.QueryDelegatorDelegationsRequest{DelegatorAddr: address})
	if err != nil {
		return nil, err
	}

	coins := sdk.Coins{}
	for _, delegation := range response.DelegationResponses {
		coins = coins.Add(delegation.Balance)
	}

	return coins, nil
}

func (p *passthroughGRPCClient) DelegatorUnbondingDelegations(ctx context.Context, address string) (sdk.Coins, error) {
	response, err := p.stakingQueryClient.DelegatorUnbondingDelegations(ctx, &staking.QueryDelegatorUnbondingDelegationsRequest{DelegatorAddr: address})
	if err != nil {
		return nil, err
	}

	coins := sdk.Coins{}
	for _, delegation := range response.UnbondingResponses {
		for _, entry := range delegation.Entries {
			coins = coins.Add(sdk.Coin{Denom: defaultBondDenom, Amount: entry.Balance})
		}
	}

	return coins, nil
}

func (p *passthroughGRPCClient) UserPositionsBalances(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error) {
	response, err := p.concentratedLiquidityQueryClient.UserPositions(ctx, &concentratedLiquidity.UserPositionsRequest{Address: address})
	if err != nil {
		return nil, nil, err
	}

	pooledCoins := sdk.Coins{}
	rewardCoins := sdk.Coins{}

	for _, position := range response.Positions {
		pooledCoins = pooledCoins.Add(position.Asset0)
		pooledCoins = pooledCoins.Add(position.Asset1)
		rewardCoins = rewardCoins.Add(position.ClaimableSpreadRewards...)
		rewardCoins = rewardCoins.Add(position.ClaimableIncentives...)
	}

	return pooledCoins, rewardCoins, nil
}
