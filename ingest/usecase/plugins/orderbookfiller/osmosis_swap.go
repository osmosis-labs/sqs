package orderbookfiller

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

	cometrpc "github.com/cometbft/cometbft/rpc/client/http"
	coretypes "github.com/cometbft/cometbft/rpc/core/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/cosmos/ibc-go/v7/testing/simapp"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
)

var (
	chainID = "osmosis-1"

	RPC       = "http://127.0.0.1:26657"
	LCD       = "http://127.0.0.1:1317"
	Denom     = "uosmo"
	NobleUSDC = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"

	encodingConfig = simapp.MakeTestEncodingConfig()
)

type AccountInfo struct {
	Sequence      string `json:"sequence"`
	AccountNumber string `json:"account_number"`
}

type AccountResult struct {
	Account AccountInfo `json:"account"`
}

type msgContext struct {
	adjustedGasUsed uint64
	msg             sdk.Msg

	maxTxFeeCap osmomath.Dec
}

func getInitialSequence(address string) (uint64, uint64) {
	resp, err := httpGet(LCD + "/cosmos/auth/v1beta1/accounts/" + address)
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
	Timeout: 10 * time.Second, // Adjusted timeout to 10 seconds
	Transport: &http.Transport{
		MaxIdleConns:        100,              // Increased maximum idle connections
		MaxIdleConnsPerHost: 10,               // Increased maximum idle connections per host
		IdleConnTimeout:     90 * time.Second, // Increased idle connection timeout
		TLSHandshakeTimeout: 10 * time.Second, // Increased TLS handshake timeout
	},
}

func httpGet(url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

func (o *orderbookFillerIngestPlugin) executeTx(ctx blockContext) (response *coretypes.ResultBroadcastTx, txbody string, err error) {
	key := o.keyring.GetKey()
	keyBytes := key.Bytes()

	privKey := &secp256k1.PrivKey{Key: keyBytes}
	// Create a new TxBuilder.
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	quoteScalingFactor, err := o.tokensUseCase.GetChainScalingFactorByDenomMut(o.defaultQuoteDenom)
	if err != nil {
		return nil, "", err
	}

	txFeeCap := osmomath.NewBigIntFromUint64(ctx.txContext.adjustedGasUsedTotal).ToDec().MulMut(ctx.baseFeeUSDCScaled).QuoMut(osmomath.BigDecFromDec(quoteScalingFactor))

	if txFeeCap.Dec().GT(ctx.txContext.maxTxFeeCap) {
		return nil, "", fmt.Errorf("tx fee capitalization %s, is greater than max allowed %s", txFeeCap, ctx.txContext.maxTxFeeCap)
	}

	txFeeUosmo := ctx.baseFee.Mul(osmomath.NewIntFromUint64(ctx.txContext.adjustedGasUsedTotal).ToLegacyDec()).Ceil().TruncateInt()
	feecoin := sdk.NewCoin(Denom, txFeeUosmo)

	err = txBuilder.SetMsgs(ctx.getSDKMsgs()...)
	if err != nil {
		return nil, "", err
	}

	txBuilder.SetGasLimit(ctx.txContext.adjustedGasUsedTotal)
	txBuilder.SetFeeAmount(sdk.NewCoins(feecoin))
	txBuilder.SetTimeoutHeight(0)

	// First round: we gather all the signer infos. We use the "set empty
	// signature" hack to do that.
	accSequence, accNumber := getInitialSequence(o.keyring.GetAddress().String())
	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  encodingConfig.TxConfig.SignModeHandler().DefaultMode(),
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
		encodingConfig.TxConfig.SignModeHandler().DefaultMode(), signerData,
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

	resp, err := BroadcastTransaction(txJSONBytes, RPC)
	if err != nil {
		return nil, "", fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	if resp.Code != 0 {
		return nil, "", fmt.Errorf("failed to broadcast transaction: %s", resp.Log)
	}

	return resp, string(txJSONBytes), nil
}

func (o *orderbookFillerIngestPlugin) simulateSwapExactAmountIn(ctx blockContext, tokenIn sdk.Coin, route []domain.RoutablePool) (msgContext, error) {
	poolManagerRoute := make([]poolmanagertypes.SwapAmountInRoute, len(route))
	for i, r := range route {
		poolManagerRoute[i] = poolmanagertypes.SwapAmountInRoute{
			PoolId:        r.GetId(),
			TokenOutDenom: r.GetTokenOutDenom(),
		}
	}

	swapMsg := &poolmanagertypes.MsgSwapExactAmountIn{
		Sender:            o.keyring.GetAddress().String(),
		Routes:            poolManagerRoute,
		TokenIn:           tokenIn,
		TokenOutMinAmount: tokenIn.Amount.Add(osmomath.OneInt()),
	}

	// Estimate transaction
	gasResult, adjustedGasUsed, err := o.simulateMsgs([]sdk.Msg{swapMsg})
	if err != nil {
		return msgContext{}, err
	}

	msgSwapExactAmountInResponse := poolmanagertypes.MsgSwapExactAmountInResponse{}

	if err := msgSwapExactAmountInResponse.Unmarshal(gasResult.Result.MsgResponses[0].Value); err != nil {
		return msgContext{}, err
	}

	if msgSwapExactAmountInResponse.TokenOutAmount.IsNil() {
		return msgContext{}, fmt.Errorf("token out amount is nil")
	}

	// Ensure that it is profitable without accounting for tx fees
	diff := msgSwapExactAmountInResponse.TokenOutAmount.Sub(tokenIn.Amount)
	if diff.IsNegative() {
		return msgContext{}, fmt.Errorf("token out amount is less than or equal to token in amount")
	}

	// Base denom price
	price := ctx.prices.GetPriceForDenom(tokenIn.Denom, o.defaultQuoteDenom)
	if price.IsZero() {
		return msgContext{}, fmt.Errorf("price for %s is zero", tokenIn.Denom)
	}

	// Compute capitalization
	diffCap := ctx.liquidityPricer.PriceCoin(sdk.Coin{Denom: baseDenom, Amount: diff}, price)

	return msgContext{
		// diff:            diff,
		maxTxFeeCap:     diffCap,
		adjustedGasUsed: adjustedGasUsed,
		msg:             swapMsg,
	}, nil
}

func (o *orderbookFillerIngestPlugin) simulateMsgs(msgs []sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
	accSeq, accNum := getInitialSequence(o.keyring.GetAddress().String())

	txFactory := tx.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig.TxConfig)
	txFactory = txFactory.WithAccountNumber(accNum)
	txFactory = txFactory.WithSequence(accSeq)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(1.05)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := tx.CalculateGas(o.passthroughGRPCClient.GetChainGRPCClient(), txFactory, msgs...)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}

func BroadcastTransaction(txBytes []byte, rpcEndpoint string) (*coretypes.ResultBroadcastTx, error) {
	cmtCli, err := cometrpc.New(rpcEndpoint, "/websocket")
	if err != nil {
		log.Fatal(err)
	}

	t := tmtypes.Tx(txBytes)

	ctx := context.Background()
	res, err := cmtCli.BroadcastTxSync(ctx, t)
	if err != nil {
		fmt.Println(err)
		fmt.Println("error at broadcast")
		return nil, err
	}

	fmt.Println("other: ", res.Data)
	fmt.Println("log: ", res.Log)
	fmt.Println("code: ", res.Code)
	fmt.Println("code: ", res.Codespace)
	fmt.Println("txid: ", res.Hash)

	return res, nil
}
