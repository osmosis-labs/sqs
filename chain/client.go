package chain

import (
	"context"
	"strconv"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/viper"
	"github.com/tendermint/tendermint/rpc/client/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"

	clpoolmodel "github.com/osmosis-labs/osmosis/v19/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v19/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v19/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v19/x/gamm/pool-models/stableswap"
	poolmanagerqueryproto "github.com/osmosis-labs/osmosis/v19/x/poolmanager/client/queryproto"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v19/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	grpctypes "github.com/cosmos/cosmos-sdk/types/grpc"
)

type Client interface {
	GetLatestHeight(ctx context.Context) (uint64, error)

	GetAllPools(ctx context.Context, desiredHeight uint64) ([]domain.PoolI, error)
}

type chainClient struct {
	context   client.Context
	rpcClient *http.HTTP
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

// GetAllPools queries and returns all chain pools
func (c chainClient) GetAllPools(ctx context.Context, desiredHeight uint64) ([]domain.PoolI, error) {
	queryClient := poolmanagerqueryproto.NewQueryClient(c.context.GRPCClient)
	var metadata runtime.ServerMetadata
	metadata.HeaderMD = make(map[string][]string)
	metadata.HeaderMD.Append(grpctypes.GRPCBlockHeightHeader, strconv.FormatUint(desiredHeight, 10))
	response, err := queryClient.AllPools(ctx, &poolmanagerqueryproto.AllPoolsRequest{}, grpc.Header(&metadata.HeaderMD), grpc.Trailer(&metadata.TrailerMD))
	if err != nil {
		return nil, err
	}

	pools := make([]domain.PoolI, 0, len(response.Pools))

	// Deserialize pools
	for _, any := range response.Pools {
		var pool poolmanagertypes.PoolI
		err := c.context.Codec.UnpackAny(any, &pool)
		if err != nil {
			return nil, err
		}

		pools = append(pools, pool)
	}

	return pools, nil
}

func init() {
	viper.SetConfigFile(`config.json`)
	err := viper.ReadInConfig()
	if err != nil {
		panic(err)
	}
}
