package usecase

import (
	"context"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"
	concentratedLiquidity "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/client/queryproto"
	lockup "github.com/osmosis-labs/osmosis/v25/x/lockup/types"

	"github.com/osmosis-labs/sqs/domain/mvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type passthroughUseCase struct {
	poolsUseCase mvc.PoolsUsecase

	bankQueryClient banktypes.QueryClient
	stakingQueryClient staking.QueryClient
	lockupQueryClient lockup.QueryClient
	concentratedLiquidityQueryClient concentratedLiquidity.QueryClient
}

var _ mvc.PassthroughUsecase = &passthroughUseCase{}

// NewPassThroughUsecase Creates a passthrough use case
func NewPassThroughUsecase(grpcURI string, puc mvc.PoolsUsecase) (mvc.PassthroughUsecase, error){
	grpcClient, err := grpc.Dial(grpcURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	
	return &passthroughUseCase{
		poolsUseCase: puc,

		bankQueryClient: banktypes.NewQueryClient(grpcClient),
		stakingQueryClient: staking.NewQueryClient(grpcClient),
		lockupQueryClient: lockup.NewQueryClient(grpcClient),
		concentratedLiquidityQueryClient: concentratedLiquidity.NewQueryClient(grpcClient),
	}, nil
}

func (p *passthroughUseCase) GetAccountCoinsTotal(ctx context.Context, address string) (sdk.Coins, error) {
	coins := sdk.NewCoins();
 	
  // Bank balances including GAMM shares
	allBalancesRes, err := p.bankQueryClient.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{
		Address: address,
	})
	if err != nil {
		return nil, err
	}
	
	for _, balance := range allBalancesRes.Balances {
		if strings.HasPrefix(balance.Denom, "gamm") {
			// calc underlying coins from gamm shares
			splitDenom := strings.Split(balance.Denom, "/")
			poolID := splitDenom[len(splitDenom)-1]
			poolIDInt, err := strconv.ParseInt(poolID, 10, 64)
			if err != nil {
				return nil, err
			}

			exitCoins, err := p.poolsUseCase.CalcExitCFMMPool(uint64(poolIDInt), balance.Amount)
			if err != nil {
				return nil, err
			}
			coins = coins.Add(exitCoins...)
		} else {
			coins = coins.Add(balance)
		}
	}

	// Staking
	delegatedRes, err := p.stakingQueryClient.DelegatorDelegations(ctx, &staking.QueryDelegatorDelegationsRequest{
		DelegatorAddr: address,
	})
	if err != nil {
		return nil, err
	}
	undelegationRes, err := p.stakingQueryClient.DelegatorUnbondingDelegations(ctx, &staking.QueryDelegatorUnbondingDelegationsRequest{
		DelegatorAddr: address,
	})
	if err != nil {
		return nil, err
	}

	for _, delegation := range delegatedRes.DelegationResponses {
		coins = coins.Add(delegation.Balance)
	}

	for _, undelegationEntry := range undelegationRes.UnbondingResponses {
		for _, undelegation := range undelegationEntry.Entries {
			coins = coins.Add(sdk.NewCoin("uosmo", undelegation.Balance))
		}
	}

	// User locked assets including GAMM shares
	locked, err := p.lockupQueryClient.AccountLockedCoins(ctx, &lockup.AccountLockedCoinsRequest{
		Owner: address,
	})

	for _, lockedCoin := range locked.Coins {
		// calc underlying coins from GAMM shares, only expect gamm shares
		if strings.HasPrefix(lockedCoin.Denom, "gamm") {
			splitDenom := strings.Split(lockedCoin.Denom, "/")
			poolID := splitDenom[len(splitDenom)-1]
			poolIDInt, err := strconv.ParseInt(poolID, 10, 64)
			if err != nil {
				return nil, err
			}

			exitCoins, err := p.poolsUseCase.CalcExitCFMMPool(uint64(poolIDInt), lockedCoin.Amount)
			if err != nil {
				return nil, err
			}
			coins = coins.Add(exitCoins...)
		} else if !strings.HasPrefix(lockedCoin.Denom, "cl") {
			coins = coins.Add(lockedCoin)
		}
	}

	// Concentrated liquidity positions
	positions, err := p.concentratedLiquidityQueryClient.UserPositions(ctx, &concentratedLiquidity.UserPositionsRequest{
		Address: address,
	})

	for _, position := range positions.Positions {
		coins = coins.Add(position.Asset0)
		coins = coins.Add(position.Asset1)
		coins = coins.Add(position.ClaimableSpreadRewards...)
		coins = coins.Add(position.ClaimableIncentives...)
	}

	return coins, nil
} 