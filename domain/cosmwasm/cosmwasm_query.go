package cosmwasmdomain

import (
	"context"
	"time"

	"github.com/osmosis-labs/osmosis/v28/ingest/types/json"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	"github.com/osmosis-labs/sqs/domain"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// queryTimeout bounds a single smart contract query. Callers often pass a background
// context, so without it a stalled node connection would block the caller indefinitely.
// A caller deadline that is sooner still applies.
const queryTimeout = 10 * time.Second

type CosmWasmPoolsParams struct {
	Config                domain.CosmWasmPoolRouterConfig
	WasmClient            wasmtypes.QueryClient
	ScalingFactorGetterCb domain.ScalingFactorGetterCb
}

// QueryCosmwasmContract queries the cosmwasm contract given the contract address, request and response
// Returns error if fails to query the contract, serialize request or deserialize response.
func QueryCosmwasmContract[T any, K any](ctx context.Context, wasmClient wasmtypes.QueryClient, contractAddress string, cosmWasmRequest T, cosmWasmResponse K) error {
	// Marshal the message
	bz, err := json.Marshal(cosmWasmRequest)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

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
