package tx

import (
	txfeestypes "github.com/osmosis-labs/osmosis/v26/x/txfees/types"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"

	gogogrpc "github.com/cosmos/gogoproto/grpc"
)

// CalculateGasFunc defines the function signature for calculating gas for a transaction.
// It is used only for testing.
type CalculateGasFunc = calculateGasFunc

// SetCalculateGasFunc sets the function used to calculate gas for a transaction.
// This is only used for testing.
func SetCalculateGasFunc(fn CalculateGasFunc) {
	calculateGas = fn
}

// SetTxServiceClient sets the tx service client used by the tx package.
// This is only used for testing.
func SetTxServiceClient(client txtypes.ServiceClient) {
	newTxServiceClient = func(gogogrpc.ClientConn) txtypes.ServiceClient {
		return client
	}
}

// SetTxFeesClient sets the txfees client used by the tx package.
// This is only used for testing.
func SetTxFeesClient(client txfeestypes.QueryClient) {
	newTxFeesClient = func(gogogrpc.ClientConn) txfeestypes.QueryClient {
		return client
	}
}
