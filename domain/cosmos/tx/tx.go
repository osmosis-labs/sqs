package tx

import (
	"context"

	cosmosClient "github.com/cosmos/cosmos-sdk/client"

	"github.com/cosmos/cosmos-sdk/client/tx"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/osmosis-labs/osmosis/osmomath"
	txfeestypes "github.com/osmosis-labs/osmosis/v26/x/txfees/types"
	"github.com/osmosis-labs/sqs/delivery/grpc"
	"github.com/osmosis-labs/sqs/domain/keyring"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"

	"github.com/osmosis-labs/osmosis/v26/app/params"
)

type Account struct {
	Sequence      uint64
	AccountNumber uint64
}

// TODO:
// SimulateMsgs
func SimulateMsgs(
	grpcClient *grpc.Client,
	encodingConfig params.EncodingConfig,
	account Account,
	chainID string,
	msgs []sdk.Msg,
) (*txtypes.SimulateResponse, uint64, error) {
	txFactory := tx.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig.TxConfig)
	txFactory = txFactory.WithAccountNumber(account.AccountNumber)
	txFactory = txFactory.WithSequence(account.Sequence)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(1.05)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := tx.CalculateGas(
		grpcClient,
		txFactory,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}

func BuildTx(ctx context.Context,grpcClient *grpc.Client, keyring keyring.Keyring, encodingConfig params.EncodingConfig, account Account, chainID string, msg ...sdk.Msg) (cosmosClient.TxBuilder, error) {
	key := keyring.GetKey()
	privKey := &secp256k1.PrivKey{Key: key.Bytes()}

	// Create and sign the transaction
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(msg...)
	if err != nil {
		return nil, err
	}

	_, gas, err := SimulateMsgs(
		grpcClient,
		encodingConfig,
		account,
		chainID,
		msg,
	)
	if err != nil {
		return nil, err
	}
	txBuilder.SetGasLimit(gas)

	feecoin, err := CalculateFeeCoin(ctx, grpcClient, gas)
	if err != nil {
		return nil, err
	}

	txBuilder.SetFeeAmount(sdk.NewCoins(feecoin))

	sigV2 := BuildSignatures(privKey.PubKey(), nil, account.Sequence)
	err = txBuilder.SetSignatures(sigV2)
	if err != nil {
		return nil, err
	}

	signerData := BuildSignerData(chainID, account.AccountNumber, account.Sequence)

	signed, err := tx.SignWithPrivKey(
		ctx,
		signingtypes.SignMode_SIGN_MODE_DIRECT, signerData,
		txBuilder, privKey, encodingConfig.TxConfig, account.Sequence)
	if err != nil {
		return nil, err
	}

	err = txBuilder.SetSignatures(signed)
	if err != nil {
		return nil, err
	}

	return txBuilder, nil
}

// SendTx broadcasts a transaction to the chain, returning the result and error.
func SendTx(ctx context.Context, grpcConn *grpc.Client, txBytes []byte) (*sdk.TxResponse, error) {
	// Broadcast the tx via gRPC. We create a new client for the Protobuf Tx service.
	txClient := txtypes.NewServiceClient(grpcConn)

	// We then call the BroadcastTx method on this client.
	resp, err := txClient.BroadcastTx(
		ctx,
		&txtypes.BroadcastTxRequest{
			Mode:    txtypes.BroadcastMode_BROADCAST_MODE_SYNC,
			TxBytes: txBytes, // Proto-binary of the signed transaction
		},
	)
	if err != nil {
		return nil, err
	}

	return resp.TxResponse, nil
}

func BuildSignatures(publicKey cryptotypes.PubKey, signature []byte, sequence uint64) signingtypes.SignatureV2 {
	return signingtypes.SignatureV2{
		PubKey: publicKey,
		Data: &signingtypes.SingleSignatureData{
			SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
			Signature: signature,
		},
		Sequence: sequence,
	}
}
func BuildSignerData(chainID string, accountNumber, sequence uint64) authsigning.SignerData {
	return authsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accountNumber,
		Sequence:      sequence,
	}
}
func CalculateFeeCoin(ctx context.Context, grpcConn *grpc.Client, gas uint64) (sdk.Coin, error) {
	client := txfeestypes.NewQueryClient(grpcConn)

	queryBaseDenomResponse, err := client.BaseDenom(ctx, &txfeestypes.QueryBaseDenomRequest{})
	if err != nil {
		return sdk.Coin{}, err
	}

	queryEipBaseFeeResponse, err := client.GetEipBaseFee(ctx, &txfeestypes.QueryEipBaseFeeRequest{})
	if err != nil {
		return sdk.Coin{}, err
	}

	feeAmount := CalculateFeeAmount(queryEipBaseFeeResponse.BaseFee, gas)

	return sdk.NewCoin(queryBaseDenomResponse.BaseDenom, feeAmount), nil
}

// CalculateFeeAmount calculates the fee based on gas and gas price
func CalculateFeeAmount(baseFee osmomath.Dec, gas uint64) osmomath.Int {
	return baseFee.MulInt64(int64(gas)).Ceil().TruncateInt()
}
