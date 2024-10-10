package claimbot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/osmosis-labs/sqs/delivery/grpc"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

	"github.com/osmosis-labs/osmosis/v26/app"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	chainID = "osmosis-1"

	RPC = "localhost:9090"
	LCD = "http://127.0.0.1:1317"

	encodingConfig = app.MakeEncodingConfig()
)

// init overrides LCD and RPC endpoints from environment variables if those are set.
func init() {
	if rpc := os.Getenv("OSMOSIS_RPC_ENDPOINT"); len(rpc) > 0 {
		RPC = rpc
	}

	if lcd := os.Getenv("OSMOSIS_LCD_ENDPOINT"); len(lcd) > 0 {
		LCD = lcd
	}
}

// sendBatchClaimTx prepares and sends a batch claim transaction to the blockchain.
// It builds the transaction, signs it, and broadcasts it to the network.
func sendBatchClaimTx(
	ctx context.Context,
	keyring keyring.Keyring,
	grpcClient *grpc.Client,
	accountQueryClient authtypes.QueryClient,
	contractAddress string,
	claims orderbookdomain.Orders,
) (*sdk.TxResponse, error) {
	address := keyring.GetAddress().String()

	account, err := getAccount(ctx, accountQueryClient, address)
	if err != nil {
		return nil, err
	}

	msgBytes, err := prepareBatchClaimMsg(claims)
	if err != nil {
		return nil, err
	}

	msg := buildExecuteContractMsg(address, contractAddress, msgBytes)

	tx, err := sqstx.BuildTx(ctx, grpcClient, keyring, encodingConfig, account, chainID, msg)
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	txBytes, err := encodingConfig.TxConfig.TxEncoder()(tx.GetTx())
	if err != nil {
		return nil, fmt.Errorf("failed to encode transaction: %w", err)
	}

	// Broadcast the transaction
	return sqstx.SendTx(ctx, grpcClient, txBytes)
}

// getAccount retrieves account information for a given address.
func getAccount(ctx context.Context, accountQueryClient authtypes.QueryClient, address string) (sqstx.Account, error) {
	account, err := accountQueryClient.GetAccount(ctx, address)
	if err != nil {
		return sqstx.Account{}, fmt.Errorf("failed to get account: %w", err)
	}
	return sqstx.Account{
		Sequence:      account.Account.Sequence,
		AccountNumber: account.Account.AccountNumber,
	}, nil
}

// prepareBatchClaimMsg creates a JSON-encoded batch claim message from the provided orders.
func prepareBatchClaimMsg(claims orderbookdomain.Orders) ([]byte, error) {
	orders := make([][]int64, len(claims))
	for i, claim := range claims {
		orders[i] = []int64{claim.TickId, claim.OrderId}
	}

	batchClaim := struct {
		BatchClaim struct {
			Orders [][]int64 `json:"orders"`
		} `json:"batch_claim"`
	}{
		BatchClaim: struct {
			Orders [][]int64 `json:"orders"`
		}{
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
