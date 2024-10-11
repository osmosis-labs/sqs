package mocks

import (
	"github.com/osmosis-labs/sqs/domain/keyring"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ keyring.Keyring = &Keyring{}

type Keyring struct {
	GetKeyFunc     func() secp256k1.PrivKey
	GetAddressFunc func() sdk.AccAddress
	GetPubKeyFunc  func() cryptotypes.PubKey
}

func (m *Keyring) GetKey() secp256k1.PrivKey {
	if m.GetKeyFunc != nil {
		return m.GetKeyFunc()
	}
	panic("unimplemented")
}

func (m *Keyring) GetAddress() sdk.AccAddress {
	if m.GetAddressFunc != nil {
		return m.GetAddressFunc()
	}
	panic("unimplemented")
}

func (m *Keyring) GetPubKey() cryptotypes.PubKey {
	if m.GetPubKeyFunc != nil {
		return m.GetPubKeyFunc()
	}
	panic("unimplemented")
}
