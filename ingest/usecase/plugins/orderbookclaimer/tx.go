package orderbookclaimer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	// "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v26/app"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	chainID = "osmosis-1"

	RPC       = "localhost:9090"
	LCD       = "http://127.0.0.1:1317"
	Denom     = "uosmo"
	NobleUSDC = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"

	encodingConfig = app.MakeEncodingConfig()
)

func (o *orderbookClaimerIngestPlugin) sendBatchClaimTx(contractAddress string, claims []Claim) error {
	key := o.keyring.GetKey()
	keyBytes := key.Bytes()
	privKey := &secp256k1.PrivKey{Key: keyBytes}

	account := o.keyring

	txConfig := encodingConfig.TxConfig

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

	// Create and sign the transaction
	txBuilder := txConfig.NewTxBuilder()

	msg := wasmtypes.MsgExecuteContract{
		Sender:   account.GetAddress().String(),
		Contract: contractAddress,
		Msg:      msgBytes,
		Funds:    sdk.NewCoins(),
	}

	err = txBuilder.SetMsgs(&msg)
	if err != nil {
		return fmt.Errorf("failed to set messages: %w", err)
	}

	// Query gas price from chain
	gasPrice, err := osmomath.NewDecFromStr("0.025") // Example gas price, adjust as necessary
	if err != nil {
		return err
	}

	_, gas, err := o.simulateMsgs(context.TODO(), []sdk.Msg{&msg})

	// Calculate the fee based on gas and gas price
	feeAmount := gasPrice.MulInt64(int64(gas)).Ceil().TruncateInt64()

	fmt.Println("fee amount", feeAmount)
	// Create the final fee structure
	feecoin := sdk.NewCoin("uosmo", osmomath.NewInt(feeAmount))

	accountSequence, accountNumber := getInitialSequence(context.TODO(), o.keyring.GetAddress().String())

	txBuilder.SetGasLimit(gas)
	txBuilder.SetFeeAmount(sdk.NewCoins(feecoin))

	signMode := encodingConfig.TxConfig.SignModeHandler().DefaultMode()
	protoSignMode, _ := authsigning.APISignModeToInternal(signMode)

	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  protoSignMode,
			Signature: nil,
		},
		Sequence: accountSequence,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return fmt.Errorf("failed to set signatures: %w", err)
	}

	signerData := authsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accountNumber,
		Sequence:      accountSequence,
	}

	signed, err := tx.SignWithPrivKey(
		context.TODO(),
		protoSignMode, signerData,
		txBuilder, privKey, encodingConfig.TxConfig, accountSequence)
	if err != nil {
		return fmt.Errorf("failed to sing transaction: %w", err)
	}

	err = txBuilder.SetSignatures(signed)
	if err != nil {
		return fmt.Errorf("failed to set signatures: %w", err)
	}

	// Broadcast the transaction
	txBytes, err := txConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return fmt.Errorf("failed to encode transaction: %w", err)
	}

	// Generate a JSON string.
	txJSONBytes, err := encodingConfig.TxConfig.TxJSONEncoder()(txBuilder.GetTx())
	if err != nil {
		return err
	}

	err = sendTx(context.TODO(), txBytes)

	log.Println("txJSON", string(txJSONBytes), err)

	return nil
}

// You'll need to define the Claim struct and Config variables
type Claim struct {
	TickID  int64
	OrderID int64
}

// Config should contain all the necessary configuration variables
var Config struct {
	CHAIN_ID      string
	RPC_ENDPOINT  string
	TX_KEY        string
	TX_GAS        int64
	TX_FEE_DENOM  string
	TX_FEE_AMOUNT int64
}

type AccountInfo struct {
	Sequence      string `json:"sequence"`
	AccountNumber string `json:"account_number"`
}

type AccountResult struct {
	Account AccountInfo `json:"account"`
}

func getInitialSequence(ctx context.Context, address string) (uint64, uint64) {
	resp, err := httpGet(ctx, LCD+"/cosmos/auth/v1beta1/accounts/"+address)
	if err != nil {
		log.Printf("Failed to get initial sequence: %v", err)
		return 0, 0
	}

	var accountRes AccountResult
	err = json.Unmarshal(resp, &accountRes)
	if err != nil {
		log.Printf("Failed to unmarshal account result: %v", err)
		return 0, 0
	}

	seqint, err := strconv.ParseUint(accountRes.Account.Sequence, 10, 64)
	if err != nil {
		log.Printf("Failed to convert sequence to int: %v", err)
		return 0, 0
	}

	accnum, err := strconv.ParseUint(accountRes.Account.AccountNumber, 10, 64)
	if err != nil {
		log.Printf("Failed to convert account number to int: %v", err)
		return 0, 0
	}

	return seqint, accnum
}

var httpClient = &http.Client{
	Timeout:   10 * time.Second, // Adjusted timeout to 10 seconds
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

func httpGet(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		netErr, ok := err.(net.Error)
		if ok && netErr.Timeout() {
			log.Printf("Request to %s timed out, continuing...", url)
			return nil, nil
		}
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// broadcastTransaction broadcasts a transaction to the chain.
// Returning the result and error.
func sendTx(ctx context.Context, txBytes []byte) error {
	// Create a connection to the gRPC server.
	grpcConn, err := grpc.NewClient(
		RPC, // Or your gRPC server address.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}

	defer grpcConn.Close()

	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx
	// service.
	txClient := txtypes.NewServiceClient(grpcConn)
	// We then call the BroadcastTx method on this client.
	grpcRes, err := txClient.BroadcastTx(
		ctx,
		&txtypes.BroadcastTxRequest{
			Mode:    txtypes.BroadcastMode_BROADCAST_MODE_SYNC,
			TxBytes: txBytes, // Proto-binary of the signed transaction, see previous step.
		},
	)
	if err != nil {
		return err
	}

	fmt.Printf("claim TxResponse: %#v\n", grpcRes.TxResponse) // Should be `0` if the tx is successful

	return nil
}

func (o *orderbookClaimerIngestPlugin) simulateMsgs(ctx context.Context, msgs []sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
	grpcConn, err := grpc.NewClient(
		RPC, // Or your gRPC server address.
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, 0, err
	}

	defer grpcConn.Close()

	accSeq, accNum := getInitialSequence(ctx, o.keyring.GetAddress().String())

	txFactory := tx.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig.TxConfig)
	txFactory = txFactory.WithAccountNumber(accNum)
	txFactory = txFactory.WithSequence(accSeq)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(1.02)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := tx.CalculateGas(
		grpcConn,
		txFactory,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}
