package mocks

import (
	"encoding/hex"

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
	panic("Keyring.GetKey(): unimplemented")
}

func (m *Keyring) WithGetKey(key string) {
	m.GetKeyFunc = func() secp256k1.PrivKey {
		return m.GenPrivKey(key)
	}
}

func (m *Keyring) GetAddress() sdk.AccAddress {
	if m.GetAddressFunc != nil {
		return m.GetAddressFunc()
	}
	panic("Keyring.GetAddress(): unimplemented")
}

func (m *Keyring) WithGetAddress(address string) {
	m.GetAddressFunc = func() sdk.AccAddress {
		return sdk.AccAddress(address)
	}
}

func (m *Keyring) GetPubKey() cryptotypes.PubKey {
	if m.GetPubKeyFunc != nil {
		return m.GetPubKeyFunc()
	}
	panic("Keyring.GetPubKey(): unimplemented")
}

func (m *Keyring) GenPrivKey(key string) secp256k1.PrivKey {
	bz, _ := hex.DecodeString(key)
	return secp256k1.PrivKey{Key: bz}
}
