package pools

import (
	"context"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/osmosis-labs/sqs/sqsdomain/json"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// initializeWasmClient initializes the wasm client given the node URI
// Returns error if fails to initialize the client
func initializeWasmClient(grpcGatewayEndpoint string) (wasmtypes.QueryClient, error) {
	grpcClient, err := grpc.NewClient(grpcGatewayEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}

	wasmClient := wasmtypes.NewQueryClient(grpcClient)

	return wasmClient, nil
}

// queryCosmwasmContract queries the cosmwasm contract given the contract address, request and response
// Returns error if fails to query the contract, serialize request or deserialize response.
func queryCosmwasmContract[T any, K any](ctx context.Context, wasmClient wasmtypes.QueryClient, contractAddress string, cosmWasmRequest T, cosmWasmResponse K) error {
	// Marshal the message
	bz, err := json.Marshal(cosmWasmRequest)
	if err != nil {
		return err
	}

	// Query the pool
	queryResponse, err := wasmClient.SmartContractState(ctx, &wasmtypes.QuerySmartContractStateRequest{
		Address:   contractAddress,
		QueryData: bz,
	}, grpc.Header(&metadata.MD{}))
	if err != nil {
		return err
	}

	if err := json.Unmarshal(queryResponse.Data, cosmWasmResponse); err != nil {
		return err
	}

	return nil
}
