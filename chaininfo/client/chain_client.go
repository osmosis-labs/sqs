package client

import (
	"context"

	clpoolmodel "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/model"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v28/x/gamm/pool-models/stableswap"

	rpchttp "github.com/cometbft/cometbft/rpc/client/http"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

type Client interface {
	GetLatestHeight(ctx context.Context) (uint64, error)
	GetStatus(ctx context.Context) (*ctypes.ResultStatus, error)
}

type chainClient struct {
	rpcClient *rpchttp.HTTP
}

func NewClient(ctx context.Context, chainID string, nodeURI string, timeout uint) (Client, error) {
	rpcClient, err := rpchttp.NewWithTimeout(nodeURI, "/websocket", timeout)
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

// GetStatus returns the status of the chain client
func (c chainClient) GetStatus(ctx context.Context) (*ctypes.ResultStatus, error) {
	statusResult, err := c.rpcClient.Status(ctx)
	if err != nil {
		return nil, err
	}

	return statusResult, nil
}

// IsConnected returns error if fails to connect to client. Nil otherwise
func (c chainClient) GetLatestHeight(ctx context.Context) (uint64, error) {
	statusResult, err := c.GetStatus(ctx)
	if err != nil {
		return 0, err
	}

	latestBlockHeight := statusResult.SyncInfo.LatestBlockHeight

	return uint64(latestBlockHeight), nil
}
