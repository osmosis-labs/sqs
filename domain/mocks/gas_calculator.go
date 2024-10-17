package mocks

import (
	txclient "github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
)

type GasCalculator struct {
	CalculateGasFunc func(txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error)
}

func (m *GasCalculator) CalculateGas(txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
	if m.CalculateGasFunc != nil {
		return m.CalculateGasFunc(txf, msgs...)
	}

	panic("GasCalculator.CalculateGasFunc not implemented")
}

func (m *GasCalculator) WithCalculateGas(response *txtypes.SimulateResponse, n uint64, err error) {
	m.CalculateGasFunc = func(txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
		return response, n, err
	}
}
