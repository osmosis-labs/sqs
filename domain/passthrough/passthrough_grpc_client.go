package passthroughdomain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	query "github.com/cosmos/cosmos-sdk/types/query"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distribution "github.com/cosmos/cosmos-sdk/x/distribution/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	math "github.com/osmosis-labs/osmosis/osmomath"
	concentratedLiquidity "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/client/queryproto"
	lockup "github.com/osmosis-labs/osmosis/v28/x/lockup/types"
	polarisgrpc "github.com/osmosis-labs/sqs/delivery/grpc"
	"google.golang.org/grpc"
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

	// DelegationTotalRewards returns the total unclaimed staking rewards accrued of the user with the given address.
	DelegationRewards(ctx context.Context, address string) (sdk.Coins, error)

	GetChainGRPCClient() grpc.ClientConnInterface
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
	distributionClient               distribution.QueryClient

	chainGRPCClient grpc.ClientConnInterface
}

const (
	defaultBondDenom = "uosmo"
)

var (
	zero = math.ZeroInt()
)

func NewPassthroughGRPCClient(grpcURI string) (PassthroughGRPCClient, error) {
	grpcClient, err := polarisgrpc.NewClient(grpcURI)
	if err != nil {
		return nil, err
	}

	return &passthroughGRPCClient{
		bankQueryClient:                  banktypes.NewQueryClient(grpcClient),
		stakingQueryClient:               staking.NewQueryClient(grpcClient),
		lockupQueryClient:                lockup.NewQueryClient(grpcClient),
		concentratedLiquidityQueryClient: concentratedLiquidity.NewQueryClient(grpcClient),
		distributionClient:               distribution.NewQueryClient(grpcClient),

		chainGRPCClient: grpcClient,
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
	return paginateRequest(ctx, func(ctx context.Context, pageRequest *query.PageRequest) (*query.PageResponse, sdk.Coins, error) {
		response, err := p.bankQueryClient.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{Address: address, Pagination: pageRequest})
		if err != nil {
			return nil, nil, err
		}
		return response.Pagination, response.Balances, nil
	})
}

func (p *passthroughGRPCClient) DelegatorDelegations(ctx context.Context, address string) (sdk.Coins, error) {
	return paginateRequest(ctx, func(ctx context.Context, pageRequest *query.PageRequest) (*query.PageResponse, sdk.Coins, error) {
		response, err := p.stakingQueryClient.DelegatorDelegations(ctx, &staking.QueryDelegatorDelegationsRequest{DelegatorAddr: address, Pagination: pageRequest})
		if err != nil {
			return nil, nil, err
		}
		coin := sdk.Coin{Denom: defaultBondDenom, Amount: zero}
		for _, delegation := range response.DelegationResponses {
			coin = coin.Add(delegation.Balance)
		}
		return response.Pagination, sdk.Coins{coin}, nil
	})
}

func (p *passthroughGRPCClient) DelegatorUnbondingDelegations(ctx context.Context, address string) (sdk.Coins, error) {
	return paginateRequest(ctx, func(ctx context.Context, pageRequest *query.PageRequest) (*query.PageResponse, sdk.Coins, error) {
		response, err := p.stakingQueryClient.DelegatorUnbondingDelegations(ctx, &staking.QueryDelegatorUnbondingDelegationsRequest{DelegatorAddr: address, Pagination: pageRequest})
		if err != nil {
			return nil, nil, err
		}
		coin := sdk.Coin{Denom: defaultBondDenom, Amount: zero}
		for _, delegation := range response.UnbondingResponses {
			for _, entry := range delegation.Entries {
				coin.Amount = coin.Amount.Add(entry.Balance)
			}
		}
		return response.Pagination, sdk.Coins{coin}, nil
	})
}

func (p *passthroughGRPCClient) UserPositionsBalances(ctx context.Context, address string) (sdk.Coins, sdk.Coins, error) {
	var (
		response = &concentratedLiquidity.UserPositionsResponse{
			Pagination: &query.PageResponse{},
		}
		isFirstRequest = true
		pooledCoins    = sdk.Coins{}
		rewardCoins    = sdk.Coins{}
		err            error
		pageRequest    *query.PageRequest
	)

	for isFirstRequest || response.Pagination.NextKey != nil {
		if !isFirstRequest {
			pageRequest = &query.PageRequest{Key: response.Pagination.NextKey}
		}

		response, err = p.concentratedLiquidityQueryClient.UserPositions(ctx, &concentratedLiquidity.UserPositionsRequest{Address: address, Pagination: pageRequest})
		if err != nil {
			return nil, nil, err
		}

		for _, position := range response.Positions {
			pooledCoins = pooledCoins.Add(position.Asset0)
			pooledCoins = pooledCoins.Add(position.Asset1)
			rewardCoins = rewardCoins.Add(position.ClaimableSpreadRewards...)
			rewardCoins = rewardCoins.Add(position.ClaimableIncentives...)
		}

		isFirstRequest = false
	}

	return pooledCoins, rewardCoins, nil
}

func (p *passthroughGRPCClient) DelegationRewards(ctx context.Context, address string) (sdk.Coins, error) {
	response, err := p.distributionClient.DelegationTotalRewards(
		ctx,
		&distribution.QueryDelegationTotalRewardsRequest{DelegatorAddress: address},
	)
	if err != nil {
		return nil, err
	}

	var rewardCoins = sdk.Coins{}
	for _, v := range response.GetTotal() {
		rewardCoins = append(rewardCoins, sdk.Coin{Denom: v.Denom, Amount: v.Amount.TruncateInt()})
	}

	return rewardCoins, nil
}

// GetChainGRPCClient implements PassthroughGRPCClient.
func (p *passthroughGRPCClient) GetChainGRPCClient() grpc.ClientConnInterface {
	return p.chainGRPCClient
}

func paginateRequest(ctx context.Context, fetchCoinsFn func(ctx context.Context, pageRequest *query.PageRequest) (*query.PageResponse, sdk.Coins, error)) (sdk.Coins, error) {
	var (
		isFirstRequest = true
		allCoins       = sdk.Coins{}
		pageRequest    = &query.PageRequest{}
	)

	for isFirstRequest || pageRequest.Key != nil {
		if !isFirstRequest {
			pageRequest = &query.PageRequest{Key: pageRequest.Key}
		}

		response, coins, err := fetchCoinsFn(ctx, pageRequest)
		if err != nil {
			return nil, err
		}

		allCoins = allCoins.Add(coins...)
		pageRequest.Key = response.NextKey
		isFirstRequest = false
	}

	return allCoins, nil
}
