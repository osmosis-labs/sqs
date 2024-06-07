package usecase

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type passthroughUseCase struct {
	bankQueryClient banktypes.QueryClient
}

var _ mvc.PassthroughUsecase = &passthroughUseCase{}

// NewPassThroughUsecase Creates a passthrough use case
func NewPassThroughUsecase(grpcURI string) (mvc.PassthroughUsecase, error){
	grpcClient, err := grpc.Dial(grpcURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	
	return &passthroughUseCase{
		bankQueryClient: banktypes.NewQueryClient(grpcClient),
	}, nil
}

func (p *passthroughUseCase) GetAccountAssetsTotal(ctx context.Context, address string) (sdk.Coins, error) {
 	res, err := p.bankQueryClient.AllBalances(ctx, &banktypes.QueryAllBalancesRequest{
		Address: address,
	})
	if err != nil {
		return nil, err
	}

	return res.Balances, nil
} 