package keyring

import (
	"fmt"
	"os"

	"github.com/99designs/keyring"
	sdkkeyring "github.com/cosmos/cosmos-sdk/crypto/keyring"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Keyring is the interface for the keyring
type Keyring interface {

	// GetKey returns the private key
	GetKey() secp256k1.PrivKey

	// GetAddress returns the address
	GetAddress() sdk.AccAddress

	// GetPubKey returns the public key
	GetPubKey() cryptotypes.PubKey
}

type keyringImpl struct {
	Key secp256k1.PrivKey
}

const (
	keyringServiceName = "cosmos"

	osmosisKeyringPathEnvName     = "OSMOSIS_KEYRING_PATH"
	osmosisKeyringPasswordEnvName = "OSMOSIS_KEYRING_PASSWORD"
	osmosisKeyringKeyNameEnvName  = "OSMOSIS_KEYRING_KEY_NAME"
)

var _ Keyring = &keyringImpl{}

func New() (*keyringImpl, error) {
	// Validate required environment variables for the keyring
	keyringPathEnv := os.Getenv(osmosisKeyringPathEnvName)
	if len(keyringPathEnv) == 0 {
		return nil, fmt.Errorf("keyring path is not set via %s", osmosisKeyringPathEnvName)
	}

	keyringPassword := os.Getenv(osmosisKeyringPasswordEnvName)
	if len(keyringPassword) == 0 {
		return nil, fmt.Errorf("keyring password is not set via %s", osmosisKeyringPasswordEnvName)
	}

	keyringKeyName := os.Getenv(osmosisKeyringKeyNameEnvName)
	if len(keyringKeyName) == 0 {
		return nil, fmt.Errorf("keyring key name not set via %s", osmosisKeyringKeyNameEnvName)
	}

	keyringConfig := keyring.Config{
		AllowedBackends: []keyring.BackendType{
			keyring.FileBackend,
		},
		ServiceName:              keyringServiceName,
		FileDir:                  keyringPathEnv,
		KeychainTrustApplication: true,
		FilePasswordFunc: func(prompt string) (string, error) {
			return keyringPassword, nil
		},
	}

	openKeyring, err := keyring.Open(keyringConfig)
	if err != nil {
		return nil, fmt.Errorf("Unable to open keyring [ %s ]: %w", keyringPathEnv, err)
	}

	// Get the keyring record
	openRecord, err := openKeyring.Get(keyringKeyName)
	if err != nil {
		return nil, fmt.Errorf("Unable to get keyring record [ %s ]: %w", keyringKeyName, err)
	}

	// Unmarshal the keyring record
	keyringRecord := new(sdkkeyring.Record)
	if err := keyringRecord.Unmarshal(openRecord.Data); err != nil {
		return nil, err
	}

	// Get the right type
	localRecord := keyringRecord.GetLocal()

	// Unmarshal the private key
	privKey := secp256k1.PrivKey{}
	if err := privKey.Unmarshal(localRecord.PrivKey.Value); err != nil {
		return nil, err
	}

	return &keyringImpl{
		Key: privKey,
	}, nil
}

func (k keyringImpl) GetKey() secp256k1.PrivKey {
	return k.Key
}

func (k keyringImpl) GetAddress() sdk.AccAddress {
	return sdk.AccAddress(k.Key.PubKey().Address())
}

func (k keyringImpl) GetPubKey() cryptotypes.PubKey {
	return k.Key.PubKey()
}
