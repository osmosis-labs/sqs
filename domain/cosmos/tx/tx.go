// Package tx provides functionality for building, simulating, and sending Cosmos SDK transactions.
package tx

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"

	txfeestypes "github.com/osmosis-labs/osmosis/v28/x/txfees/types"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

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

// CalculateFeePrice determines the appropriate fee price for a transaction based on the current base fee
// and the amount of gas used. It queries the base denomination and EIP base fee using the provided gRPC connection.
func CalculateFeePrice(ctx context.Context, client txfeestypes.QueryClient) (domain.BaseFee, error) {
	queryBaseDenomResponse, err := client.BaseDenom(ctx, &txfeestypes.QueryBaseDenomRequest{})
	if err != nil {
		return domain.BaseFee{}, err
	}

	queryEipBaseFeeResponse, err := client.GetEipBaseFee(ctx, &txfeestypes.QueryEipBaseFeeRequest{})
	if err != nil {
		return domain.BaseFee{}, err
	}

	return domain.BaseFee{
		Denom:      queryBaseDenomResponse.BaseDenom,
		CurrentFee: queryEipBaseFeeResponse.BaseFee,
	}, nil
}

// CalculateFeeAmount calculates the fee amount based on the base fee and gas used.
// It multiplies the base fee by the gas amount, rounds up to the nearest integer, and returns the result.
func CalculateFeeAmount(baseFee osmomath.Dec, gas uint64) osmomath.Int {
	return baseFee.MulInt64(int64(gas)).Ceil().TruncateInt()
}
