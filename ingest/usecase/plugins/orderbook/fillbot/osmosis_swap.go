package fillbot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"go.uber.org/zap"

	cometrpc "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/app"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/fillbot/context/block"
	msgctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/fillbot/context/msg"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	noTxFeeCheckHeightInterval = 40
)

var (
	chainID = "osmosis-1"

	RPC       = "http://127.0.0.1:26657"
	LCD       = "http://127.0.0.1:1317"
	Denom     = "uosmo"
	NobleUSDC = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"

	encodingConfig = app.MakeEncodingConfig()
)

type AccountInfo struct {
	Sequence      string `json:"sequence"`
	AccountNumber string `json:"account_number"`
}

type AccountResult struct {
	Account AccountInfo `json:"account"`
}

// init overrides LCD and RPC endpoints
// from environment variables if those are set.
func init() {
	osmosisRPCOverwrite := os.Getenv("OSMOSIS_RPC_ENDPOINT")
	if len(osmosisRPCOverwrite) > 0 {
		RPC = osmosisRPCOverwrite
	}

	osmosisLCDOverwrite := os.Getenv("OSMOSIS_LCD_ENDPOINT")
	if len(osmosisLCDOverwrite) > 0 {
		LCD = osmosisLCDOverwrite
	}
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

var client = &http.Client{
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

// executeTx executes a transaction with the given tx context and block gas price.
// It returns the response, the transaction body and an error if any.
// It waits for 5 seconds before returning.
// It returns an error and avoids executing the transaction if the tx fee capitalization is greater than the max allowed.
func (o *orderbookFillerIngestPlugin) executeTx(blockCtx blockctx.BlockCtxI) (response *coretypes.ResultBroadcastTx, txbody string, err error) {
	key := o.keyring.GetKey()
	keyBytes := key.Bytes()

	privKey := &secp256k1.PrivKey{Key: keyBytes}
	// Create a new TxBuilder.
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	quoteScalingFactor, err := o.tokensUseCase.GetChainScalingFactorByDenomMut(o.defaultQuoteDenom)
	if err != nil {
		return nil, "", err
	}

	blockGasPrice := blockCtx.GetGasPrice()
	txCtx := blockCtx.GetTxCtx()
	adjustedTxGasUsedTotal := txCtx.GetAdjustedGasUsedTotal()

	txFeeCap := osmomath.NewBigIntFromUint64(adjustedTxGasUsedTotal).ToDec().MulMut(blockGasPrice.GasPriceDefaultQuoteDenom).QuoMut(osmomath.BigDecFromDec(quoteScalingFactor))

	// We skip the fee check for every noTxFeeCheckHeightInterval blocks
	// Every 40 blocks (roughly 1 minute), batch all off-market orders and execute them
	// potentially at a loss. This is roughly 4 cents per minute assumming 3 swap messages at 0.1 uosmo per gas.
	// Which is only $57 per day
	if blockCtx.GetBlockHeight()%noTxFeeCheckHeightInterval != 0 {
		maxTxFeeCap := txCtx.GetMaxTxFeeCap()
		if txFeeCap.Dec().GT(maxTxFeeCap) {
			return nil, "", fmt.Errorf("tx fee capitalization %s, is greater than max allowed %s", txFeeCap, maxTxFeeCap)
		}
	} else {
		o.logger.Info("skipping tx fee check", zap.String("tx_fee_cap", txFeeCap.String()), zap.String("max_txf_fee_cap", txCtx.GetMaxTxFeeCap().String()), zap.Uint64("block_height", blockCtx.GetBlockHeight()))
	}

	txFeeUosmo := blockGasPrice.GasPrice.Mul(osmomath.NewIntFromUint64(adjustedTxGasUsedTotal).ToLegacyDec()).Ceil().TruncateInt()
	feecoin := sdk.NewCoin(Denom, txFeeUosmo)

	err = txBuilder.SetMsgs(txCtx.GetSDKMsgs()...)
	if err != nil {
		return nil, "", err
	}

	txBuilder.SetGasLimit(adjustedTxGasUsedTotal)
	txBuilder.SetFeeAmount(sdk.NewCoins(feecoin))
	txBuilder.SetTimeoutHeight(0)

	// First round: we gather all the signer infos. We use the "set empty
	// signature" hack to do that.
	accSequence, accNumber := getInitialSequence(blockCtx.AsGoCtx(), o.keyring.GetAddress().String())
	signMode := encodingConfig.TxConfig.SignModeHandler().DefaultMode()
	protoSignMode, _ := authsigning.APISignModeToInternal(signMode)

	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  protoSignMode,
			Signature: nil,
		},
		Sequence: accSequence,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		fmt.Println("error setting signatures")
		return nil, "", err
	}

	signerData := authsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accNumber,
		Sequence:      accSequence,
	}

	signed, err := tx.SignWithPrivKey(
		blockCtx.AsGoCtx(),
		protoSignMode, signerData,
		txBuilder, privKey, encodingConfig.TxConfig, accSequence)
	if err != nil {
		fmt.Println("couldn't sign")
		return nil, "", err
	}

	err = txBuilder.SetSignatures(signed)
	if err != nil {
		return nil, "", err
	}

	// Generate a JSON string.
	txJSONBytes, err := encodingConfig.TxConfig.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		fmt.Println(err)
		return nil, "", err
	}

	defer func() {
		// Wait for block inclusion with buffer to avoid sequence mismatch
		time.Sleep(5 * time.Second)
	}()

	resp, err := broadcastTransaction(blockCtx.AsGoCtx(), txJSONBytes, RPC)
	if err != nil {
		return nil, "", fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if resp.Code != 0 {
		return nil, "", fmt.Errorf("failed to broadcast transaction: %s", resp.Log)
	}

	o.logger.Info("executed transaction: ", zap.Uint32("code", resp.Code), zap.String("hash", string(resp.Hash)), zap.String("log", resp.Log), zap.String("codespace", resp.Codespace))

	return resp, string(txJSONBytes), nil
}

func (o *orderbookFillerIngestPlugin) simulateSwapExactAmountIn(ctx blockctx.BlockCtxI, tokenIn sdk.Coin, route []domain.RoutablePool) (msgctx.MsgContextI, error) {
	poolManagerRoute := make([]poolmanagertypes.SwapAmountInRoute, len(route))
	for i, r := range route {
		poolManagerRoute[i] = poolmanagertypes.SwapAmountInRoute{
			PoolId:        r.GetId(),
			TokenOutDenom: r.GetTokenOutDenom(),
		}
	}

	// Note that we lower the slippage bound, allowing losses.
	// We still do profitability checks for all swaps > $5 of value down below.
	// However, we allow for losses in the case of small swaps.
	// This is to ensure proper filling. The losses are bounded by:
	// $5 * (1 - 0.9995) = $0.002
	slippageBound := tokenIn.Amount.ToLegacyDec().Mul(osmomath.MustNewDecFromStr("0.9995")).TruncateInt()

	swapMsg := &poolmanagertypes.MsgSwapExactAmountIn{
		Sender:            o.keyring.GetAddress().String(),
		Routes:            poolManagerRoute,
		TokenIn:           tokenIn,
		TokenOutMinAmount: slippageBound,
	}

	// Estimate transaction
	gasResult, adjustedGasUsed, err := o.simulateMsgs(ctx.AsGoCtx(), []sdk.Msg{swapMsg})
	if err != nil {
		return nil, err
	}

	msgSwapExactAmountInResponse := poolmanagertypes.MsgSwapExactAmountInResponse{}

	if err := msgSwapExactAmountInResponse.Unmarshal(gasResult.Result.MsgResponses[0].Value); err != nil {
		return nil, err
	}

	if msgSwapExactAmountInResponse.TokenOutAmount.IsNil() {
		return nil, fmt.Errorf("token out amount is nil")
	}

	// Base denom price
	blockPrices := ctx.GetPrices()
	price := blockPrices.GetPriceForDenom(tokenIn.Denom, o.defaultQuoteDenom)
	if price.IsZero() {
		return nil, fmt.Errorf("price for %s is zero", tokenIn.Denom)
	}

	// For small unprofitable fills, we allow for a small loss.
	diffCap := osmomath.MustNewDecFromStr("0.005")
	if o.liquidityPricer.PriceCoin(tokenIn, price).GTE(osmomath.MustNewDecFromStr("5")) {
		// Otherwise, we compute the capitalization difference precisely.
		// Ensure that it is profitable without accounting for tx fees
		diff := msgSwapExactAmountInResponse.TokenOutAmount.Sub(tokenIn.Amount)
		if diff.IsNegative() {
			return nil, fmt.Errorf("token out amount is less than or equal to token in amount")
		}

		// Compute capitalization
		diffCap = o.liquidityPricer.PriceCoin(sdk.Coin{Denom: orderbookplugindomain.BaseDenom, Amount: diff}, price)
	}

	msgCtx := msgctx.New(diffCap, adjustedGasUsed, swapMsg)

	return msgCtx, nil
}

func (o *orderbookFillerIngestPlugin) simulateMsgs(ctx context.Context, msgs []sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
	accSeq, accNum := getInitialSequence(ctx, o.keyring.GetAddress().String())

	txFactory := tx.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig.TxConfig)
	txFactory = txFactory.WithAccountNumber(accNum)
	txFactory = txFactory.WithSequence(accSeq)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(1.02)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := tx.CalculateGas(
		o.passthroughGRPCClient.GetChainGRPCClient(),
		txFactory,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}

// broadcastTransaction broadcasts a transaction to the chain.
// Returning the result and error.
func broadcastTransaction(ctx context.Context, txBytes []byte, rpcEndpoint string) (*coretypes.ResultBroadcastTx, error) {
	cmtCli, err := cometrpc.New(rpcEndpoint, "/websocket")
	if err != nil {
		return nil, err
	}

	t := tmtypes.Tx(txBytes)

	res, err := cmtCli.BroadcastTxSync(ctx, t)
	if err != nil {
		return nil, err
	}

	return res, nil
}
