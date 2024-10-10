package orderbookclaimer

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/v26/app"

	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
)

var (
	chainID = "osmosis-1"

	RPC   = "localhost:9090"
	LCD   = "http://127.0.0.1:1317"
	Denom = "uosmo"

	encodingConfig = app.MakeEncodingConfig()
)

func (o *orderbookClaimerIngestPlugin) sendBatchClaimTx(contractAddress string, claims []Claim) error {
	account, err := o.accountQueryClient.GetAccount(context.TODO(), o.keyring.GetAddress().String())
	if err != nil {
		// TODO
	}

	// Prepare the message
	orders := make([][]int64, len(claims))
	for i, claim := range claims {
		orders[i] = []int64{claim.TickID, claim.OrderID}
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
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	msg := wasmtypes.MsgExecuteContract{
		Sender:   o.keyring.GetAddress().String(),
		Contract: contractAddress,
		Msg:      msgBytes,
		Funds:    sdk.NewCoins(),
	}

	tx, err := sqstx.BuildTx(context.TODO(), o.grpcClient, o.keyring, encodingConfig, sqstx.Account{
		Sequence:      account.Account.Sequence,
		AccountNumber: account.Account.AccountNumber,
	}, chainID, &msg)


	// Broadcast the transaction
	txBytes, err := encodingConfig.TxConfig.TxEncoder()(tx.GetTx())
	if err != nil {
		return fmt.Errorf("failed to encode transaction: %w", err)
	}

	// Generate a JSON string.
	txJSONBytes, err := encodingConfig.TxConfig.TxJSONEncoder()(tx.GetTx())
	if err != nil {
		return err
	}

	log.Println("txJSON", string(txJSONBytes), err)

	defer func() {
		// Wait for block inclusion with buffer to avoid sequence mismatch
		time.Sleep(5 * time.Second)
	}()

	txresp, err := sqstx.SendTx(context.TODO(), o.grpcClient, txBytes)

	log.Printf("txres %#v : %s", txresp, err)


	return nil
}

// TODO
// You'll need to define the Claim struct and Config variables
type Claim struct {
	TickID  int64
	OrderID int64
}
