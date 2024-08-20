package main

import (
	"context"
	"fmt"

	tenderminapi "cosmossdk.io/api/cosmos/base/tendermint/v1beta1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	chainsimulatedomain "github.com/osmosis-labs/sqs/domain/chainsimulate"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

type ChainClient struct {
	grpcClient grpc.ClientConnInterface
	chainLCD   string
}

func (c *ChainClient) SimulateSwapExactAmountOut(ctx context.Context, address string, routes []poolmanagertypes.SwapAmountOutRoute, tokenOut sdk.Coin, slippageBound osmomath.Int) (osmomath.Int, error) {
	swapMsg := &poolmanagertypes.MsgSwapExactAmountOut{
		Sender:           address,
		Routes:           routes,
		TokenOut:         tokenOut,
		TokenInMaxAmount: slippageBound,
	}

	resp, _, err := chainsimulatedomain.SimulateMsgs(ctx, c.grpcClient, c.chainLCD, address, []sdk.Msg{swapMsg})
	if err != nil {
		return osmomath.Int{}, err
	}

	respMsgs := resp.Result.MsgResponses
	if len(respMsgs) != 1 {
		return osmomath.Int{}, fmt.Errorf("expected 1 message response, got %d", len(respMsgs))
	}

	msgSwapExactAmountInResponse := poolmanagertypes.MsgSwapExactAmountOutResponse{}

	if err := msgSwapExactAmountInResponse.Unmarshal(respMsgs[0].Value); err != nil {
		return osmomath.Int{}, err
	}

	return msgSwapExactAmountInResponse.TokenInAmount, nil
}

func createGRPCGatewayClient(ctx context.Context, grpcGatewayEndpoint string, chainLCD string) (*ChainClient, error) {
	grpcClient, err := grpc.NewClient(grpcGatewayEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}

	tendermintGRPCClient := tenderminapi.NewServiceClient(grpcClient)
	if _, err := tendermintGRPCClient.GetLatestBlock(ctx, &tenderminapi.GetLatestBlockRequest{}); err != nil {
		return nil, err
	}

	return &ChainClient{
		grpcClient: grpcClient,
		chainLCD:   chainLCD,
	}, nil
}

func constructSwapAmountOutRoute(routes []domain.SplitRoute) ([]poolmanagertypes.SwapAmountOutRoute, error) {
	if len(routes) != 1 {
		return nil, fmt.Errorf("invalid split route length must be 1, was %d", len(routes))
	}

	chainExactOutRoutes := []poolmanagertypes.SwapAmountOutRoute{}

	for _, pool := range routes[0].GetPools() {
		chainExactOutRoutes = append(chainExactOutRoutes, poolmanagertypes.SwapAmountOutRoute{
			PoolId:       pool.GetId(),
			TokenInDenom: pool.GetTokenInDenom(),
		})
	}

	reverseSlice(chainExactOutRoutes)

	return chainExactOutRoutes, nil
}
