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
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/cosmos/ibc-go/v7/testing/simapp"
	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanager "github.com/osmosis-labs/osmosis/v25/x/poolmanager/module"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
	"github.com/osmosis-labs/sqs/domain"
)

var (
	RPC       = "http://127.0.0.1:26657"
	LCD       = "http://127.0.0.1:1317"
	Denom     = "uosmo"
	NobleUSDC = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	// TODO: Delete this
	// minReqArbProfitForSwap = sdk.MustNewDecFromStr("0.005") // 0.5% profit
	// cdc = codec.NewProtoCodec(codectypes.NewInterfaceRegistry())
)

type AccountInfo struct {
	Sequence      string `json:"sequence"`
	AccountNumber string `json:"account_number"`
}

type AccountResult struct {
	Account AccountInfo `json:"account"`
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

func (o *orderbookFillerIngestPlugin) swapExactAmountIn(tokenIn sdk.Coin, tokenOutAmount osmomath.Int, route []domain.RoutablePool) (response *coretypes.ResultBroadcastTx, txbody string, err error) {
	// TODO: Remove this atomic bool, just here for testing
	o.swapDone.Store(true)
	sequence, accnum := getInitialSequence(o.keyring.GetAddress().String())
	chainID := "osmosis-1"
	key := o.keyring.GetKey()
	keyBytes := key.Bytes()

	privKey := &secp256k1.PrivKey{Key: keyBytes}

	encodingConfig := simapp.MakeTestEncodingConfig()
	// encodingConfig.Marshaler = cdc

	// TODO: Delete all of this once we confirm the API is what we want
	//
	// price, err := o.tokensUseCase.GetPrices(ctx, []string{tokenIn.Denom}, []string{NobleUSDC}, domain.ChainPricingSourceType)
	// if err != nil {
	// 	return nil, "", fmt.Errorf("failed to get prices: %w", err)
	// }
	// // TODO: Im not sure if this is actual USD price or if it needs to be normalized? Or actually maybe it does not matter.
	// tokenInPrice := price.GetPriceForDenom(tokenIn.Denom, NobleUSDC)
	// tokenInUSDC := tokenIn.Amount.ToLegacyDec().Mul(tokenInPrice.Dec())

	// poolIdsSwappingAcross := make([]uint64, len(route))
	// for i, r := range route {
	// 	poolIdsSwappingAcross[i] = r.PoolId
	// }

	// // Get the swap fee for each pool, and determine the total usdc value of fees to be paid
	// totalSwapFeeUSDC := sdk.ZeroDec()
	// for _, poolId := range poolIdsSwappingAcross {
	// 	tokenIn := tokenInUSDC.Sub(totalSwapFeeUSDC)
	// 	pool, err := o.poolsUseCase.GetPool(poolId)
	// 	if err != nil {
	// 		return nil, "", fmt.Errorf("failed to get pool: %w", err)
	// 	}

	// 	spreadFactor := pool.GetSQSPoolModel().SpreadFactor
	// 	spreadFactorUSDC := spreadFactor.Mul(tokenIn)

	// 	takerFeeForPair, err := o.routerUseCase.GetTakerFee(poolId)
	// 	if err != nil {
	// 		return nil, "", fmt.Errorf("failed to get taker fee: %w", err)
	// 	}
	// 	// TODO: Assumes we are using two denom pools every time
	// 	takerFee := takerFeeForPair[0].TakerFee
	// 	takerFeeUSDC := takerFee.Mul(tokenIn.Sub(spreadFactorUSDC))
	// 	totalSwapFeeUSDC = totalSwapFeeUSDC.Add(takerFeeUSDC).Add(spreadFactorUSDC)
	// }

	// // Turn totalSwapFeeUSDC into the amount of tokenIn this represents
	// totalSwapFee := totalSwapFeeUSDC.Quo(tokenInPrice.Dec()).TruncateInt()

	// // Calculate the minimum token out amount considering the required profit and total swap fees
	// // TODO: Maybe we need to consider the gas fees here as well, but rn we are coding for 0.5 percent profit,
	// // which should mean we don't need to consider gas fees
	// tokenInWithFees := tokenIn.Amount.Add(totalSwapFee)
	// minTokenOutAmount := tokenInWithFees.ToLegacyDec().Mul(sdk.OneDec().Sub(minReqArbProfitForSwap)).TruncateInt()

	// Register types
	poolm := poolmanager.AppModuleBasic{}
	poolm.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	// Create a new TxBuilder.
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	// Using the []domain.RoutablePool route, create a []poolmanagertypes.SwapAmountInRoute route
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
		TokenOutMinAmount: tokenOutAmount,
	}

	msg := []sdk.Msg{swapMsg}

	// set messages
	err = txBuilder.SetMsgs(msg...)
	if err != nil {
		return nil, "", err
	}

	// // Estimate gas limit based on transaction size
	// txSize := 0
	// for _, m := range msg {
	// 	txSize += m.Size()
	// }
	// txSize := msg
	// gasLimit := uint64((txSize * config.Bytes) + config.BaseGas)
	gasLimit := uint64(1700000)
	txBuilder.SetGasLimit(gasLimit)

	// Calculate fee based on gas limit and a fixed gas price
	gasPrice := sdk.NewDecCoinFromDec(Denom, sdk.NewDecWithPrec(25, 4)) // 0.00051 token per gas unit
	feeAmount := gasPrice.Amount.MulInt64(int64(gasLimit)).RoundInt()
	feecoin := sdk.NewCoin(Denom, feeAmount)
	txBuilder.SetFeeAmount(sdk.NewCoins(feecoin))
	txBuilder.SetTimeoutHeight(0)

	// First round: we gather all the signer infos. We use the "set empty
	// signature" hack to do that.
	sigV2 := signing.SignatureV2{
		PubKey: privKey.PubKey(),
		Data: &signing.SingleSignatureData{
			SignMode:  encodingConfig.TxConfig.SignModeHandler().DefaultMode(),
			Signature: nil,
		},
		Sequence: sequence,
	}

	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		fmt.Println("error setting signatures")
		return nil, "", err
	}

	signerData := authsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accnum,
		Sequence:      sequence,
	}

	signed, err := tx.SignWithPrivKey(
		encodingConfig.TxConfig.SignModeHandler().DefaultMode(), signerData,
		txBuilder, privKey, encodingConfig.TxConfig, sequence)
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

	resp, err := BroadcastTransaction(txJSONBytes, RPC)
	if err != nil {
		return nil, "", fmt.Errorf("failed to broadcast transaction: %w", err)
	}

	return resp, string(txJSONBytes), nil
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
