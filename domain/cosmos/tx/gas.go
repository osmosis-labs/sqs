package tx

import (
	txclient "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"

	gogogrpc "github.com/cosmos/gogoproto/grpc"
)

// GasCalculator is an interface for calculating gas for a transaction.
type GasCalculator interface {
	CalculateGas(txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error)
}

// NewGasCalculator creates a new GasCalculator instance.
func NewGasCalculator(clientCtx gogogrpc.ClientConn) GasCalculator {
	return &TxGasCalulator{
		clientCtx: clientCtx,
	}
}

// TxGasCalulator is a GasCalculator implementation that uses simulated transactions to calculate gas.
type TxGasCalulator struct {
	clientCtx gogogrpc.ClientConn
}

// CalculateGas calculates the gas required for a transaction using the provided transaction factory and messages.
func (c *TxGasCalulator) CalculateGas(
	txf txclient.Factory,
	msgs ...sdk.Msg,
) (*txtypes.SimulateResponse, uint64, error) {
	gasResult, adjustedGasUsed, err := txclient.CalculateGas(
		c.clientCtx,
		txf,
		msgs...,
	)
	if err != nil {
		return nil, adjustedGasUsed, err
	}

	return gasResult, adjustedGasUsed, nil
}
