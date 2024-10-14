package claimbot

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/delivery/grpc"
	"github.com/osmosis-labs/sqs/domain"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Order is order alias data structure for testing purposes.
type Order = order

// ProcessOrderbooksAndGetClaimableOrders is test wrapper for processOrderbooksAndGetClaimableOrders.
// This function is exported for testing purposes.
func ProcessOrderbooksAndGetClaimableOrders(
	ctx context.Context,
	fillThreshold osmomath.Dec,
	orderbooks []domain.CanonicalOrderBooksResult,
	orderbookRepository orderbookdomain.OrderBookRepository,
	orderBookClient orderbookgrpcclientdomain.OrderBookClient,
	orderbookusecase mvc.OrderBookUsecase,
	logger log.Logger,
) []Order {
	return processOrderbooksAndGetClaimableOrders(ctx, fillThreshold, orderbooks, orderbookRepository, orderBookClient, orderbookusecase, logger)
}

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

// SendBatchClaimTx a test wrapper for sendBatchClaimTx.
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

// GetAccount is a test wrapper for getAccount.
// This function is exported for testing purposes.
func GetAccount(ctx context.Context, client authtypes.QueryClient, address string) (sqstx.Account, error) {
	return getAccount(ctx, client, address)
}

// PrepareBatchClaimMsg is a test wrapper for prepareBatchClaimMsg.
// This function is exported for testing purposes.
func PrepareBatchClaimMsg(claims orderbookdomain.Orders) ([]byte, error) {
	return prepareBatchClaimMsg(claims)
}
