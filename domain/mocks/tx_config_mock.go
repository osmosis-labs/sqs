package mocks

import (
	"cosmossdk.io/x/tx/signing"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
)

type TxConfigMock struct {
	TxEncoderFn func() types.TxEncoder
}

// MarshalSignatureJSON implements client.TxConfig.
func (t *TxConfigMock) MarshalSignatureJSON([]signingtypes.SignatureV2) ([]byte, error) {
	panic("unimplemented")
}

// NewTxBuilder implements client.TxConfig.
func (t *TxConfigMock) NewTxBuilder() client.TxBuilder {
	panic("unimplemented")
}

// SignModeHandler implements client.TxConfig.
func (t *TxConfigMock) SignModeHandler() *signing.HandlerMap {
	panic("unimplemented")
}

// SigningContext implements client.TxConfig.
func (t *TxConfigMock) SigningContext() *signing.Context {
	panic("unimplemented")
}

// TxDecoder implements client.TxConfig.
func (t *TxConfigMock) TxDecoder() types.TxDecoder {
	panic("unimplemented")
}

// TxEncoder implements client.TxConfig.
func (t *TxConfigMock) TxEncoder() types.TxEncoder {
	if t.TxEncoderFn != nil {
		return t.TxEncoderFn()
	}

	panic("unimplemented")
}

// TxJSONDecoder implements client.TxConfig.
func (t *TxConfigMock) TxJSONDecoder() types.TxDecoder {
	panic("unimplemented")
}

// TxJSONEncoder implements client.TxConfig.
func (t *TxConfigMock) TxJSONEncoder() types.TxEncoder {
	panic("unimplemented")
}

// UnmarshalSignatureJSON implements client.TxConfig.
func (t *TxConfigMock) UnmarshalSignatureJSON([]byte) ([]signingtypes.SignatureV2, error) {
	panic("unimplemented")
}

// WrapTxBuilder implements client.TxConfig.
func (t *TxConfigMock) WrapTxBuilder(types.Tx) (client.TxBuilder, error) {
	panic("unimplemented")
}

var _ client.TxConfig = &TxConfigMock{}
