package pools

import (
	"context"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/osmosis-labs/sqs/sqsdomain/json"
)

// initializeWasmClient initializes the wasm client given the node URI
// Returns error if fails to initialize the client
func initializeWasmClient(nodeURI string) (wasmtypes.QueryClient, error) {
	clientContext := client.Context{}
	httpClient, err := client.NewClientFromNode(nodeURI)
	if err != nil {
		return nil, err
	}
	clientContext = clientContext.WithClient(httpClient)
	wasmClient := wasmtypes.NewQueryClient(clientContext)

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
	})
	if err != nil {
		return err
	}

	if err := json.Unmarshal(queryResponse.Data, cosmWasmResponse); err != nil {
		return err
	}

	return nil
}
