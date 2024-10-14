package claimbot

import (
	"context"

	"github.com/osmosis-labs/sqs/delivery/grpc"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// buildTxFunc is a function signature for buildTx.
// This type is used only for testing purposes.
type BuildTx = buildTxFunc

// SetBuildTx is used to override function that constructs a transaction.
// This function is used only for testing purposes.
func SetBuildTx(fn buildTxFunc) {
	buildTx = fn
}

// SendTxFunc is an alias for the sendTxFunc.
// This type is used only for testing purposes.
type SendTxFunc = sendTxFunc

// SetSendTx is used to override function that sends a transaction to the blockchain.
// This function is used only for testing purposes.
func SetSendTx(fn sendTxFunc) {
	sendTx = fn
}

// SendBatchClaimTx prepares and sends a batch claim transaction to the blockchain.
// This function is used only for testing purposes.
func SendBatchClaimTx(
	ctx context.Context,
	keyring keyring.Keyring,
	grpcClient *grpc.Client,
	accountQueryClient authtypes.QueryClient,
	contractAddress string,
	claims orderbookdomain.Orders,
) (*sdk.TxResponse, error) {
	return sendBatchClaimTx(ctx, keyring, grpcClient, accountQueryClient, contractAddress, claims)
}

// GetAccount retrieves account information for a given address.
// This function is exported for testing purposes.
func GetAccount(ctx context.Context, client authtypes.QueryClient, address string) (sqstx.Account, error) {
	return getAccount(ctx, client, address)
}

// PrepareBatchClaimMsg prepares a batch claim message for the claimbot.
// This function is exported for testing purposes.
func PrepareBatchClaimMsg(claims orderbookdomain.Orders) ([]byte, error) {
	return prepareBatchClaimMsg(claims)
}
