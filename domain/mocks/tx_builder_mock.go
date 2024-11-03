package mocks

import (
	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/x/auth/signing"

	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
)

type TxBuilderMock struct {
	AddAuxSignerDataFn func(tx.AuxSignerData) error

	GetTxFn func() signing.Tx

	SetFeeAmountFn func(amount types.Coins)

	SetFeeGranterFn func(feeGranter types.AccAddress)

	SetFeePayerFn func(feePayer types.AccAddress)

	SetGasLimitFn func(limit uint64)

	SetMemoFn func(memo string)

	SetMsgsFn func(msgs ...interface {
		ProtoMessage()
		Reset()
		String() string
	}) error

	SetSignaturesFn func(signatures ...signingtypes.SignatureV2) error

	SetTimeoutHeightFn func(height uint64)
}

// AddAuxSignerData implements client.TxBuilder.
func (t *TxBuilderMock) AddAuxSignerData(auxSignerData tx.AuxSignerData) error {
	if t.AddAuxSignerDataFn != nil {
		return t.AddAuxSignerDataFn(auxSignerData)
	}
	return nil
}

// GetTx implements client.TxBuilder.
func (t *TxBuilderMock) GetTx() signing.Tx {
	if t.GetTxFn != nil {
		return t.GetTxFn()
	}

	panic("unimplemented")
}

// SetFeeAmount implements client.TxBuilder.
func (t *TxBuilderMock) SetFeeAmount(amount types.Coins) {
	if t.SetFeeAmountFn != nil {
		t.SetFeeAmountFn(amount)
	}

	panic("unimplemented")
}

// SetFeeGranter implements client.TxBuilder.
func (t *TxBuilderMock) SetFeeGranter(feeGranter types.AccAddress) {
	if t.SetFeeGranterFn != nil {
		t.SetFeeGranterFn(feeGranter)
	}

	panic("unimplemented")
}

// SetFeePayer implements client.TxBuilder.
func (t *TxBuilderMock) SetFeePayer(feePayer types.AccAddress) {
	if t.SetFeePayerFn != nil {
		t.SetFeePayerFn(feePayer)
	}

	panic("unimplemented")
}

// SetGasLimit implements client.TxBuilder.
func (t *TxBuilderMock) SetGasLimit(limit uint64) {
	if t.SetGasLimitFn != nil {
		t.SetGasLimitFn(limit)
	}

	panic("unimplemented")
}

// SetMemo implements client.TxBuilder.
func (t *TxBuilderMock) SetMemo(memo string) {
	if t.SetMemoFn != nil {
		t.SetMemoFn(memo)
	}

	panic("unimplemented")
}

// SetMsgs implements client.TxBuilder.
func (t *TxBuilderMock) SetMsgs(msgs ...interface {
	ProtoMessage()
	Reset()
	String() string
}) error {
	if t.SetMsgsFn != nil {
		return t.SetMsgsFn(msgs...)
	}

	panic("unimplemented")
}

// SetSignatures implements client.TxBuilder.
func (t *TxBuilderMock) SetSignatures(signatures ...signingtypes.SignatureV2) error {
	if t.SetSignaturesFn != nil {
		return t.SetSignaturesFn(signatures...)
	}

	panic("unimplemented")
}

// SetTimeoutHeight implements client.TxBuilder.
func (t *TxBuilderMock) SetTimeoutHeight(height uint64) {
	if t.SetTimeoutHeightFn != nil {
		t.SetTimeoutHeightFn(height)
	}

	panic("unimplemented")
}

var _ cosmosclient.TxBuilder = &TxBuilderMock{}
