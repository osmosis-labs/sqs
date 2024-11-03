package mocks

import (
	"github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/signing"
	"github.com/cosmos/gogoproto/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var _ signing.Tx = &TxMock{}

type TxMock struct {
	FeeGranterFn       func() []byte
	FeePayerFn         func() []byte
	GetFeeFn           func() sdk.Coins
	GetGasFn           func() uint64
	GetMemoFn          func() string
	GetMsgsFn          func() []proto.Message
	GetMsgsV2Fn        func() ([]protoreflect.ProtoMessage, error)
	GetPubKeysFn       func() ([]types.PubKey, error)
	GetSignaturesV2Fn  func() ([]signingtypes.SignatureV2, error)
	GetSignersFn       func() ([][]byte, error)
	GetTimeoutHeightFn func() uint64
	ValidateBasicFn    func() error
}

// FeeGranter implements signing.Tx.
func (t *TxMock) FeeGranter() []byte {
	if t.FeeGranterFn != nil {
		return t.FeeGranterFn()
	}

	panic("unimplemented")
}

// FeePayer implements signing.Tx.
func (t *TxMock) FeePayer() []byte {
	if t.FeePayerFn != nil {
		return t.FeePayerFn()
	}

	panic("unimplemented")
}

// GetFee implements signing.Tx.
func (t *TxMock) GetFee() sdk.Coins {
	if t.GetFeeFn != nil {
		return t.GetFeeFn()
	}

	panic("unimplemented")
}

// GetGas implements signing.Tx.
func (t *TxMock) GetGas() uint64 {
	if t.GetGasFn != nil {
		return t.GetGasFn()
	}

	panic("unimplemented")
}

// GetMemo implements signing.Tx.
func (t *TxMock) GetMemo() string {
	if t.GetMemoFn != nil {
		return t.GetMemoFn()
	}

	panic("unimplemented")
}

// GetMsgs implements signing.Tx.
func (t *TxMock) GetMsgs() []proto.Message {
	if t.GetMsgsFn != nil {
		return t.GetMsgsFn()
	}

	panic("unimplemented")
}

// GetMsgsV2 implements signing.Tx.
func (t *TxMock) GetMsgsV2() ([]protoreflect.ProtoMessage, error) {
	if t.GetMsgsV2Fn != nil {
		return t.GetMsgsV2Fn()
	}

	panic("unimplemented")
}

// GetPubKeys implements signing.Tx.
func (t *TxMock) GetPubKeys() ([]types.PubKey, error) {
	if t.GetPubKeysFn != nil {
		return t.GetPubKeysFn()
	}

	panic("unimplemented")
}

// GetSignaturesV2 implements signing.Tx.
func (t *TxMock) GetSignaturesV2() ([]signingtypes.SignatureV2, error) {
	if t.GetSignaturesV2Fn != nil {
		return t.GetSignaturesV2Fn()
	}

	panic("unimplemented")
}

// GetSigners implements signing.Tx.
func (t *TxMock) GetSigners() ([][]byte, error) {
	if t.GetSignersFn != nil {
		return t.GetSignersFn()
	}

	panic("unimplemented")
}

// GetTimeoutHeight implements signing.Tx.
func (t *TxMock) GetTimeoutHeight() uint64 {
	if t.GetTimeoutHeightFn != nil {
		return t.GetTimeoutHeightFn()
	}

	panic("unimplemented")
}

// ValidateBasic implements signing.Tx.
func (t *TxMock) ValidateBasic() error {
	if t.ValidateBasicFn != nil {
		return t.ValidateBasicFn()
	}

	panic("unimplemented")
}
