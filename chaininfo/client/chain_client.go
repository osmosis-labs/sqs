package client

import (
	"context"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	clpoolmodel "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v25/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v25/x/gamm/pool-models/stableswap"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

type Client interface {
	GetLatestHeight(ctx context.Context) (uint64, error)
	GetBalance(ctx context.Context, address string) (sdk.Coins, error)
}

type chainClient struct {
	rpcClient *rpchttp.HTTP
	grpcConn  *grpc.ClientConn
}

func NewClient(chainID string, nodeURI string, grpcURI string) (Client, error) {
	rpcClient, err := client.NewClientFromNode(nodeURI)
	if err != nil {
		return nil, err
	}

	grpcConn, err := grpc.Dial(grpcURI, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	balancer.RegisterInterfaces(interfaceRegistry)
	stableswap.RegisterInterfaces(interfaceRegistry)
	clpoolmodel.RegisterInterfaces(interfaceRegistry)
	cwpoolmodel.RegisterInterfaces(interfaceRegistry)

	return &chainClient{
		rpcClient: rpcClient,
		grpcConn:  grpcConn,
	}, nil
}

// IsConnected returns error if fails to connect to client. Nil otherwise
func (c chainClient) GetLatestHeight(ctx context.Context) (uint64, error) {
	statusResult, err := c.rpcClient.Status(ctx)
	if err != nil {
		return 0, err
	}

	latestBlockHeight := statusResult.SyncInfo.LatestBlockHeight

	return uint64(latestBlockHeight), nil
}

// GetBalance fetches the balance of a given address
func (c chainClient) GetBalance(ctx context.Context, address string) (sdk.Coins, error) {
	queryClient := banktypes.NewQueryClient(c.grpcConn)

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
