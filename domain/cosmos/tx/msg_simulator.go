package tx

import (
	"context"
	"errors"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	txclient "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/osmosis-labs/osmosis/v28/app/params"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/keyring"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"google.golang.org/grpc"

	gogogrpc "github.com/cosmos/gogoproto/grpc"
)

// MsgSimulator is an interface for calculating gas for a transaction.
type MsgSimulator interface {
	BuildTx(
		ctx context.Context,
		keyring keyring.Keyring,
		encodingConfig params.EncodingConfig,
		account *authtypes.BaseAccount,
		chainID string,
		msg ...sdk.Msg,
	) (cosmosclient.TxBuilder, error)

	// SimulateMsgs simulates the execution of the given messages and returns the simulation response,
	// adjusted gas used, and any error encountered. It uses the provided gRPC client, encoding config,
	// account details, and chain ID to create a transaction factory for the simulation.
	SimulateMsgs(
		encodingConfig cosmosclient.TxConfig,
		account *authtypes.BaseAccount,
		chainID string,
		msgs []sdk.Msg,
	) (*txtypes.SimulateResponse, uint64, error)

	// PriceMsgs simulates the execution of the given messages and returns the gas used and the fee coin,
	// which is the fee amount in the base denomination.
	PriceMsgs(
		ctx context.Context,
		encodingConfig cosmosclient.TxConfig,
		account *authtypes.BaseAccount,
		chainID string,
		msg ...sdk.Msg,
	) domain.TxFeeInfo
}

// NewMsgSimulator creates a new GasCalculator instance.
func NewMsgSimulator(clientCtx gogogrpc.ClientConn, calculateGas CalculateGasFn, memoryRouterRepository routerrepo.RouterRepository) MsgSimulator {
	return &txGasCalulator{
		clientCtx:              clientCtx,
		calculateGas:           calculateGas,
		memoryRouterRepository: memoryRouterRepository,
	}
}

// CalculateGasFn is a function type that calculates the gas for a transaction.
type CalculateGasFn func(clientCtx gogogrpc.ClientConn, txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error)

// txGasCalulator is a GasCalculator implementation that uses simulated transactions to calculate gas.
type txGasCalulator struct {
	clientCtx              grpc.ClientConnInterface
	calculateGas           CalculateGasFn
	memoryRouterRepository routerrepo.BaseFeeRepository
}

// BuildTx constructs a transaction using the provided parameters and messages.
// Returns a TxBuilder and any error encountered.
func (c *txGasCalulator) BuildTx(
	ctx context.Context,
	keyring keyring.Keyring,
	encodingConfig params.EncodingConfig,
	account *authtypes.BaseAccount,
	chainID string,
	msg ...sdk.Msg,
) (cosmosclient.TxBuilder, error) {
	key := keyring.GetKey()
	privKey := &secp256k1.PrivKey{Key: key.Bytes()}

	// Create and sign the transaction
	txBuilder := encodingConfig.TxConfig.NewTxBuilder()

	err := txBuilder.SetMsgs(msg...)
	if err != nil {
		return nil, err
	}

	priceInfo := c.PriceMsgs(ctx, encodingConfig.TxConfig, account, chainID, msg...)
	if priceInfo.Err != "" {
		return nil, errors.New(priceInfo.Err)
	}

	txBuilder.SetGasLimit(priceInfo.AdjustedGasUsed)
	txBuilder.SetFeeAmount(sdk.Coins{priceInfo.FeeCoin})

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

// SimulateMsgs implements MsgSimulator.
func (c *txGasCalulator) SimulateMsgs(encodingConfig cosmosclient.TxConfig, account *authtypes.BaseAccount, chainID string, msgs []sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
	txFactory := txclient.Factory{}
	txFactory = txFactory.WithTxConfig(encodingConfig)
	txFactory = txFactory.WithAccountNumber(account.AccountNumber)
	txFactory = txFactory.WithSequence(account.Sequence)
	txFactory = txFactory.WithChainID(chainID)
	txFactory = txFactory.WithGasAdjustment(1.15)

	// Estimate transaction
	gasResult, adjustedGasUsed, err := c.calculateGas(
		c.clientCtx,
		txFactory,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}

// PriceMsgs implements MsgSimulator.
func (c *txGasCalulator) PriceMsgs(ctx context.Context, encodingConfig cosmosclient.TxConfig, account *authtypes.BaseAccount, chainID string, msg ...sdk.Msg) domain.TxFeeInfo {
	baseFee := c.memoryRouterRepository.GetBaseFee()
	if baseFee.CurrentFee.IsNil() || baseFee.CurrentFee.IsZero() {
		return domain.TxFeeInfo{Err: "base fee is zero or nil"}
	}
	if baseFee.Denom == "" {
		return domain.TxFeeInfo{Err: "base fee denom is empty"}
	}

	_, gasAdjusted, err := c.SimulateMsgs(
		encodingConfig,
		account,
		chainID,
		msg,
	)
	if err != nil {
		return domain.TxFeeInfo{Err: err.Error(), BaseFee: baseFee.CurrentFee}
	}

	feeAmount := CalculateFeeAmount(baseFee.CurrentFee, gasAdjusted)

	return domain.TxFeeInfo{
		AdjustedGasUsed: gasAdjusted,
		FeeCoin:         sdk.Coin{Denom: baseFee.Denom, Amount: feeAmount},
		BaseFee:         baseFee.CurrentFee,
		Err:             "",
	}
}

// CalculateGas calculates the gas required for a transaction using the provided transaction factory and messages.
func CalculateGas(
	clientCtx gogogrpc.ClientConn,
	txf txclient.Factory,
	msgs ...sdk.Msg,
) (*txtypes.SimulateResponse, uint64, error) {
	gasResult, adjustedGasUsed, err := txclient.CalculateGas(
		clientCtx,
		txf,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}
