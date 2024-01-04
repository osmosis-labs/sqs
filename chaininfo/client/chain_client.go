package client

import (
	"context"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"

	clpoolmodel "github.com/osmosis-labs/osmosis/v21/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v21/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v21/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v21/x/gamm/pool-models/stableswap"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

type Client interface {
	GetLatestHeight(ctx context.Context) (uint64, error)
}

type chainClient struct {
	context   client.Context
	rpcClient *rpchttp.HTTP
}

func NewClient(chainID string, nodeURI string) (Client, error) {
	rpcClient, err := client.NewClientFromNode(nodeURI)
	if err != nil {
		return nil, err
	}

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	balancer.RegisterInterfaces(interfaceRegistry)
	stableswap.RegisterInterfaces(interfaceRegistry)
	clpoolmodel.RegisterInterfaces(interfaceRegistry)
	cwpoolmodel.RegisterInterfaces(interfaceRegistry)

	clientCtx := client.Context{}.
		WithCodec(codec.NewProtoCodec(interfaceRegistry)).
		WithChainID(chainID)

	// If grpc is enabled, configure grpc client for grpc gateway.
	grpcClient, err := grpc.Dial(
		viper.GetString(`chain.node_grpc`), // TODO: get from config
		// nolint: staticcheck
		grpc.WithInsecure(),
		grpc.WithDefaultCallOptions(
			grpc.ForceCodec(encoding.GetCodec(proto.Name)),
			grpc.MaxCallRecvMsgSize(10485760),
			grpc.MaxCallSendMsgSize(2147483647),
		),
	)
	if err != nil {
		return nil, err
	}

	clientCtx = clientCtx.WithGRPCClient(grpcClient)

	return &chainClient{
		rpcClient: rpcClient,
		context:   clientCtx,
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

func init() {
	viper.SetConfigFile(`config.json`)
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
}
