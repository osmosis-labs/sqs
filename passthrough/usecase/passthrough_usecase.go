package usecase

import (
	"context"
	"strconv"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	staking "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/osmosis-labs/sqs/domain/mvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type passthroughUseCase struct {
	poolsUseCase mvc.PoolsUsecase

	bankQueryClient banktypes.QueryClient
	stakingQueryClient staking.QueryClient
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
	}, nil
}

func (p *passthroughUseCase) GetAccountCoinsTotal(ctx context.Context, address string) (sdk.Coins, error) {
 	allBalancesRes, err := p.bankQueryClient.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{
		Address: address,
	})
	if err != nil {
		return nil, err
	}
	
	// Collect all balances from gamm shares and 
	coins := sdk.NewCoins();
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

	return coins, nil
} 