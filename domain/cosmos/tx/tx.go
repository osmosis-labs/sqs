// Package tx provides functionality for building, simulating, and sending Cosmos SDK transactions.
package tx

import (
	"context"

	"github.com/osmosis-labs/sqs/domain/keyring"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v27/app/params"
	txfeestypes "github.com/osmosis-labs/osmosis/v27/x/txfees/types"

	cosmosClient "github.com/cosmos/cosmos-sdk/client"
	txclient "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// BuildTx constructs a transaction using the provided parameters and messages.
// Returns a TxBuilder and any error encountered.
func BuildTx(
	ctx context.Context,
	keyring keyring.Keyring,
	txfeesClient txfeestypes.QueryClient,
	gasCalculator GasCalculator,
	encodingConfig params.EncodingConfig,
	account *authtypes.BaseAccount,
	chainID string,
	msg ...sdk.Msg,
) (cosmosClient.TxBuilder, error) {
	key := keyring.GetKey()
	privKey := &secp256k1.PrivKey{Key: key.Bytes()}

	// Create and sign the transaction
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(msg...)
	if err != nil {
		return nil, err
	}

	_, gas, err := SimulateMsgs(
		gasCalculator,
		encodingConfig,
		account,
		chainID,
		msg,
	)
	if err != nil {
		return nil, err
	}
	txBuilder.SetGasLimit(gas)

	feecoin, err := CalculateFeeCoin(ctx, txfeesClient, gas)
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

	signed, err := txclient.SignWithPrivKey(
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
func SendTx(ctx context.Context, txServiceClient txtypes.ServiceClient, txBytes []byte) (*sdk.TxResponse, error) {
	// We then call the BroadcastTx method on this client.
	resp, err := txServiceClient.BroadcastTx(
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

// SimulateMsgs simulates the execution of the given messages and returns the simulation response,
// adjusted gas used, and any error encountered. It uses the provided gRPC client, encoding config,
// account details, and chain ID to create a transaction factory for the simulation.
func SimulateMsgs(
	gasCalculator GasCalculator,
	encodingConfig params.EncodingConfig,
	account *authtypes.BaseAccount,
	chainID string,
	msgs []sdk.Msg,
) (*txtypes.SimulateResponse, uint64, error) {
	txFactory := txclient.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig.TxConfig)
	txFactory = txFactory.WithAccountNumber(account.AccountNumber)
	txFactory = txFactory.WithSequence(account.Sequence)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(1.15)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := gasCalculator.CalculateGas(
		txFactory,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}

// BuildSignatures creates a SignatureV2 object using the provided public key, signature, and sequence number.
// This is used in the process of building and signing transactions.
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

// BuildSignerData creates a SignerData object with the given chain ID, account number, and sequence.
// This data is used in the process of signing transactions.
func BuildSignerData(chainID string, accountNumber, sequence uint64) authsigning.SignerData {
	return authsigning.SignerData{
		ChainID:       chainID,
		AccountNumber: accountNumber,
		Sequence:      sequence,
	}
}

// CalculateFeeCoin determines the appropriate fee coin for a transaction based on the current base fee
// and the amount of gas used. It queries the base denomination and EIP base fee using the provided gRPC connection.
func CalculateFeeCoin(ctx context.Context, client txfeestypes.QueryClient, gas uint64) (sdk.Coin, error) {
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

// CalculateFeeAmount calculates the fee amount based on the base fee and gas used.
// It multiplies the base fee by the gas amount, rounds up to the nearest integer, and returns the result.
func CalculateFeeAmount(baseFee osmomath.Dec, gas uint64) osmomath.Int {
	return baseFee.MulInt64(int64(gas)).Ceil().TruncateInt()
}
