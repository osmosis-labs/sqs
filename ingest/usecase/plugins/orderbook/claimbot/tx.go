package claimbot

import (
	"context"
	"encoding/json"
	"fmt"

	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

	"github.com/osmosis-labs/osmosis/v27/app"
	txfeestypes "github.com/osmosis-labs/osmosis/v27/x/txfees/types"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
<<<<<<< HEAD
=======
	"github.com/osmosis-labs/osmosis/v26/app"
	"github.com/osmosis-labs/osmosis/v26/app/params"
>>>>>>> fcbf7b6 (refactor: decouple base fee fetching as part of quote from simulation (#550))
)

var (
	encodingConfig = app.MakeEncodingConfig()
)

// sendBatchClaimTx prepares and sends a batch claim transaction to the blockchain.
// It builds the transaction, signs it, and broadcasts it to the network.
func sendBatchClaimTx(
	ctx context.Context,
	keyring keyring.Keyring,
<<<<<<< HEAD
	txfeesClient txfeestypes.QueryClient,
	gasCalculator sqstx.GasCalculator,
=======
	msgSimulator sqstx.MsgSimulator,
>>>>>>> fcbf7b6 (refactor: decouple base fee fetching as part of quote from simulation (#550))
	txServiceClient txtypes.ServiceClient,
	chainID string,
	account *authtypes.BaseAccount,
	contractAddress string,
	claims orderbookdomain.Orders,
) (*sdk.TxResponse, error) {
<<<<<<< HEAD
=======
	return sendBatchClaimTxInternal(ctx, keyring, msgSimulator, txServiceClient, chainID, account, contractAddress, claims, defaultEncodingConfigFn)
}

// sendBatchClaimTxInternal is a helper function that prepares and sends a batch claim transaction to the blockchain.
// It takes an encoding config function as a parameter to allow for customization of the encoding config in tests.
func sendBatchClaimTxInternal(
	ctx context.Context,
	keyring keyring.Keyring,
	msgSimulator sqstx.MsgSimulator,
	txServiceClient txtypes.ServiceClient,
	chainID string,
	account *authtypes.BaseAccount,
	contractAddress string,
	claims orderbookdomain.Orders,
	getEncodingConfig func() params.EncodingConfig,
) (*sdk.TxResponse, error) {
	encodingConfig := getEncodingConfig()

>>>>>>> fcbf7b6 (refactor: decouple base fee fetching as part of quote from simulation (#550))
	address := keyring.GetAddress().String()

	msgBytes, err := prepareBatchClaimMsg(claims)
	if err != nil {
		return nil, err
	}

	msg := buildExecuteContractMsg(address, contractAddress, msgBytes)

<<<<<<< HEAD
	tx, err := sqstx.BuildTx(ctx, keyring, txfeesClient, gasCalculator, encodingConfig, account, chainID, msg)
=======
	tx, err := msgSimulator.BuildTx(ctx, keyring, encodingConfig, account, chainID, msg)
>>>>>>> fcbf7b6 (refactor: decouple base fee fetching as part of quote from simulation (#550))
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	txBytes, err := encodingConfig.TxConfig.TxEncoder()(tx.GetTx())
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction: %w", err)
	}

	return sqstx.SendTx(ctx, txServiceClient, txBytes)
}

// batchClaim represents batch claim orders message.
type batchClaim struct {
	batchClaimOrders `json:"batch_claim"`
}

// batchClaimOrders represents the orders in the batch claim message.
// Each order is represented by a pair of tick ID and order ID.
type batchClaimOrders struct {
	Orders [][]int64 `json:"orders"`
}

// prepareBatchClaimMsg creates a JSON-encoded batch claim message from the provided orders.
func prepareBatchClaimMsg(claims orderbookdomain.Orders) ([]byte, error) {
	orders := make([][]int64, len(claims))
	for i, claim := range claims {
		orders[i] = []int64{claim.TickId, claim.OrderId}
	}

	batchClaim := batchClaim{
		batchClaimOrders: batchClaimOrders{
			Orders: orders,
		},
	}

	msgBytes, err := json.Marshal(batchClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}
	return msgBytes, nil
}

// buildExecuteContractMsg constructs a message for executing a smart contract.
func buildExecuteContractMsg(address, contractAddress string, msgBytes []byte) *wasmtypes.MsgExecuteContract {
	return &wasmtypes.MsgExecuteContract{
		Sender:   address,
		Contract: contractAddress,
		Msg:      msgBytes,
		Funds:    sdk.NewCoins(),
	}
}
