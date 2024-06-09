package clients

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
)

type BankClient struct {
	grpcConn *grpc.ClientConn
}

type BankClientI interface {
	GetBalance(ctx context.Context, address string) (sdk.Coins, error)
}

func NewBankClient(grpcConn *grpc.ClientConn) *BankClient {
	return &BankClient{
		grpcConn: grpcConn,
	}
}

func (bc *BankClient) GetBalance(ctx context.Context, address string) (sdk.Coins, error) {
	queryClient := banktypes.NewQueryClient(bc.grpcConn)

	res, err := queryClient.AllBalances(
		ctx,
		&banktypes.QueryAllBalancesRequest{
			Address: address,
		},
	)

	if err != nil {
		return nil, err
	}

	if len(res.Balances) == 0 {
		return sdk.Coins{}, nil
	}

	return res.Balances, nil
}

// Ensure BankClient implements BankClientI
var _ BankClientI = &BankClient{}
