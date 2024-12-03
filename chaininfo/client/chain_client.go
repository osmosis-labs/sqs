package client

import (
	"context"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	"github.com/cosmos/cosmos-sdk/client"

	clpoolmodel "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/stableswap"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

type Client interface {
	GetLatestHeight(ctx context.Context) (uint64, error)
}

type chainClient struct {
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

	return &chainClient{
		rpcClient: rpcClient,
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
