package mocks

import (
	"context"

	"github.com/cosmos/cosmos-sdk/client"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/osmosis-labs/osmosis/v27/app/params"
	txfeestypes "github.com/osmosis-labs/osmosis/v27/x/txfees/types"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
)

type MsgSimulatorMock struct {
	BuildTxFn func(
		ctx context.Context,
		keyring keyring.Keyring,
		txfeesClient txfeestypes.QueryClient,
		encodingConfig params.EncodingConfig,
		account *authtypes.BaseAccount,
		chainID string,
		msg ...sdk.Msg,
	) (client.TxBuilder, error)

	SimulateMsgsFn func(
		encodingConfig client.TxConfig,
		account *authtypes.BaseAccount,
		chainID string,
		msgs []sdk.Msg,
	) (*txtypes.SimulateResponse, uint64, error)

	PriceMsgsFn func(
		ctx context.Context,
		txfeesClient txfeestypes.QueryClient,
		encodingConfig client.TxConfig,
		account *authtypes.BaseAccount,
		chainID string,
		msg ...sdk.Msg,
	) (uint64, sdk.Coin, error)
}

var _ sqstx.MsgSimulator = &MsgSimulatorMock{}

func (m *MsgSimulatorMock) BuildTx(ctx context.Context,
	keyring keyring.Keyring,
	txfeesClient txfeestypes.QueryClient,
	encodingConfig params.EncodingConfig,
	account *authtypes.BaseAccount,
	chainID string,
	msg ...sdk.Msg,
) (client.TxBuilder, error) {
	if m.BuildTxFn != nil {
		return m.BuildTxFn(ctx, keyring, txfeesClient, encodingConfig, account, chainID, msg...)
	}
	panic("BuildTxFn not implemented")
}

func (m *MsgSimulatorMock) SimulateMsgs(
	encodingConfig client.TxConfig,
	account *authtypes.BaseAccount,
	chainID string,
	msgs []sdk.Msg,
) (*txtypes.SimulateResponse, uint64, error) {
	if m.SimulateMsgsFn != nil {
		return m.SimulateMsgsFn(encodingConfig, account, chainID, msgs)
	}
	panic("SimulateMsgsFn not implemented")
}

// PriceMsgs implements tx.MsgSimulator.
func (m *MsgSimulatorMock) PriceMsgs(ctx context.Context, txfeesClient txfeestypes.QueryClient, encodingConfig client.TxConfig, account *authtypes.BaseAccount, chainID string, msg ...interface {
	ProtoMessage()
	Reset()
	String() string
}) (uint64, sdk.Coin, error) {
	if m.PriceMsgsFn != nil {
		return m.PriceMsgsFn(ctx, txfeesClient, encodingConfig, account, chainID, msg...)
	}
	panic("PriceMsgsFn not implemented")
}
