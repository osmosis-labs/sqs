package chainsimulatedomain

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/client/tx"
	gogogrpc "github.com/cosmos/gogoproto/grpc"
	"github.com/cosmos/ibc-go/v7/testing/simapp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	sdk "github.com/cosmos/cosmos-sdk/types"

	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
)

type AccountInfo struct {
	Sequence      string `json:"sequence"`
	AccountNumber string `json:"account_number"`
}

type AccountResult struct {
	Account AccountInfo `json:"account"`
}

var client = &http.Client{
	Timeout:   10 * time.Second, // Adjusted timeout to 10 seconds
	Transport: otelhttp.NewTransport(http.DefaultTransport),
}

const (
	chainID       = "osmosis-1"
	gasAdjustment = 1.02
)

var (
	encodingConfig = simapp.MakeTestEncodingConfig()
)

// SimulateMsgs simulates the execution of a transaction and returns the
// simulation response obtained by the query and the adjusted gas amount.
func SimulateMsgs(ctx context.Context, clientCtx gogogrpc.ClientConn, lcd, address string, msgs []sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
	accSeq, accNum := GetInitialSequence(ctx, lcd, address)

	txFactory := tx.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig.TxConfig)
	txFactory = txFactory.WithAccountNumber(accNum)
	txFactory = txFactory.WithSequence(accSeq)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(gasAdjustment)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := CalculateGas(ctx, clientCtx, txFactory, msgs...)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}

// CalculateGas simulates the execution of a transaction and returns the
// simulation response obtained by the query and the adjusted gas amount.
func CalculateGas(
	ctx context.Context,
	clientCtx gogogrpc.ClientConn, txf tx.Factory, msgs ...sdk.Msg,
) (*txtypes.SimulateResponse, uint64, error) {
	txBytes, err := txf.BuildSimTx(msgs...)
	if err != nil {
		return nil, 0, err
	}

	txSvcClient := txtypes.NewServiceClient(clientCtx)
	simRes, err := txSvcClient.Simulate(ctx, &txtypes.SimulateRequest{
		TxBytes: txBytes,
	})
	if err != nil {
		return nil, 0, err
	}

	return simRes, uint64(txf.GasAdjustment() * float64(simRes.GasInfo.GasUsed)), nil
}

// GetInitialSequence fetches the initial sequence and account number for a given address
// from the LCD endpoint. It returns the sequence and account number as uint64 values.
func GetInitialSequence(ctx context.Context, lcd string, address string) (uint64, uint64) {
	resp, err := httpGet(ctx, lcd+"/cosmos/auth/v1beta1/accounts/"+address)
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

// httpGet performs an HTTP GET request to the specified URL and returns the response body.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
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
