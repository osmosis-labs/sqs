package claimbot

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"

	txfeestypes "github.com/osmosis-labs/osmosis/v26/x/txfees/types"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
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

// SendBatchClaimTx a test wrapper for sendBatchClaimTx.
// This function is used only for testing purposes.
func SendBatchClaimTx(
	ctx context.Context,
	keyring keyring.Keyring,
	accountQueryClient authtypes.QueryClient,
	txfeesClient txfeestypes.QueryClient,
	gasCalculator sqstx.GasCalculator,
	txServiceClient txtypes.ServiceClient,
	contractAddress string,
	claims orderbookdomain.Orders,
) (*sdk.TxResponse, error) {
	return sendBatchClaimTx(ctx, keyring, accountQueryClient, txfeesClient, gasCalculator, txServiceClient, contractAddress, claims)
}

// PrepareBatchClaimMsg is a test wrapper for prepareBatchClaimMsg.
// This function is exported for testing purposes.
func PrepareBatchClaimMsg(claims orderbookdomain.Orders) ([]byte, error) {
	return prepareBatchClaimMsg(claims)
}

// GetOrderbooks is a test wrapper for getOrderbooks.
// This function is exported for testing purposes.
func GetOrderbooks(poolsUsecase mvc.PoolsUsecase, blockHeight uint64, metadata domain.BlockPoolMetadata) ([]domain.CanonicalOrderBooksResult, error) {
	return getOrderbooks(poolsUsecase, blockHeight, metadata)
}
