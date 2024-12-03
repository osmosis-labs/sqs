package tx_test

import (
	"context"
	"testing"

	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/mocks"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/app"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var (
	encodingConfig = app.MakeEncodingConfig()

	newMsg = func(sender, contract, msg string) sdk.Msg {
		return &wasmtypes.MsgExecuteContract{
			Sender:   sender,
			Contract: contract,
			Msg:      []byte(msg),
			Funds:    sdk.NewCoins(),
		}
	}
)

func TestSendTx(t *testing.T) {
	newBroadcastTxFunc := func(txResponse *txtypes.BroadcastTxResponse, err error) func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
		return func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
			return txResponse, err
		}
	}
	tests := []struct {
		name            string
		txBytes         []byte
		txServiceClient mocks.TxServiceClient
		expectedResult  *sdk.TxResponse
		expectedError   error
	}{
		{
			name:    "Successful transaction",
			txBytes: []byte("txbytes"),
			txServiceClient: mocks.TxServiceClient{
				BroadcastTxFunc: newBroadcastTxFunc(&txtypes.BroadcastTxResponse{
					TxResponse: &sdk.TxResponse{
						Code:   0,
						TxHash: "test_hash",
					},
				}, nil),
			},
			expectedResult: &sdk.TxResponse{Code: 0, TxHash: "test_hash"},
			expectedError:  nil,
		},
		{
			name:    "Error in BroadcastTx",
			txBytes: []byte("failtxbytes"),
			txServiceClient: mocks.TxServiceClient{
				BroadcastTxFunc: newBroadcastTxFunc(nil, assert.AnError),
			},
			expectedResult: nil,
			expectedError:  assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sqstx.SendTx(context.Background(), &tt.txServiceClient, tt.txBytes)

			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestBuildSignatures(t *testing.T) {
	tests := []struct {
		name        string
		publicKey   cryptotypes.PubKey
		signature   []byte
		sequence    uint64
		expectedSig signingtypes.SignatureV2
	}{
		{
			name:      "Valid signature",
			publicKey: secp256k1.GenPrivKey().PubKey(),
			signature: []byte("test signature"),
			sequence:  10,
			expectedSig: signingtypes.SignatureV2{
				PubKey: secp256k1.GenPrivKey().PubKey(),
				Data: &signingtypes.SingleSignatureData{
					SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
					Signature: []byte("test signature"),
				},
				Sequence: 10,
			},
		},
		{
			name:      "Empty signature",
			publicKey: secp256k1.GenPrivKey().PubKey(),
			signature: []byte{},
			sequence:  5,
			expectedSig: signingtypes.SignatureV2{
				PubKey: secp256k1.GenPrivKey().PubKey(),
				Data: &signingtypes.SingleSignatureData{
					SignMode:  signingtypes.SignMode_SIGN_MODE_DIRECT,
					Signature: []byte{},
				},
				Sequence: 5,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sqstx.BuildSignatures(tt.publicKey, tt.signature, tt.sequence)

			assert.Equal(t, tt.expectedSig.Sequence, result.Sequence)
			assert.Equal(t, tt.expectedSig.Data.(*signingtypes.SingleSignatureData).SignMode, result.Data.(*signingtypes.SingleSignatureData).SignMode)
			assert.Equal(t, tt.expectedSig.Data.(*signingtypes.SingleSignatureData).Signature, result.Data.(*signingtypes.SingleSignatureData).Signature)

			assert.Equal(t, tt.publicKey.Bytes(), result.PubKey.Bytes())
		})
	}
}

func TestBuildSignerData(t *testing.T) {
	tests := []struct {
		name          string
		chainID       string
		accountNumber uint64
		sequence      uint64
		expected      authsigning.SignerData
	}{
		{
			name:          "Basic test",
			chainID:       "test-chain",
			accountNumber: 1,
			sequence:      5,
			expected: authsigning.SignerData{
				ChainID:       "test-chain",
				AccountNumber: 1,
				Sequence:      5,
			},
		},
		{
			name:          "Zero values",
			chainID:       "",
			accountNumber: 0,
			sequence:      0,
			expected: authsigning.SignerData{
				ChainID:       "",
				AccountNumber: 0,
				Sequence:      0,
			},
		},
		{
			name:          "Large values",
			chainID:       "long-chain-id-123456789",
			accountNumber: 9999999,
			sequence:      9999999,
			expected: authsigning.SignerData{
				ChainID:       "long-chain-id-123456789",
				AccountNumber: 9999999,
				Sequence:      9999999,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sqstx.BuildSignerData(tt.chainID, tt.accountNumber, tt.sequence)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateFeeCoin(t *testing.T) {
	tests := []struct {
		name            string
		gas             uint64
		txFeesClient    mocks.TxFeesQueryClient
		setupMocks      func(*mocks.TxFeesQueryClient)
		expectedCoin    string
		expectedAmount  osmomath.Int
		expectedBaseFee osmomath.Dec
		expectError     bool
	}{
		{
			name: "Normal case",
			gas:  100000,
			setupMocks: func(client *mocks.TxFeesQueryClient) {
				client.WithBaseDenom("uosmo", nil)
				client.WithGetEipBaseFee("0.5", nil)
			},
			expectedCoin:    "uosmo",
			expectedAmount:  osmomath.NewInt(50000),
			expectedBaseFee: osmomath.NewDecWithPrec(5, 1),
			expectError:     false,
		},
		{
			name: "Error getting base denom",
			setupMocks: func(client *mocks.TxFeesQueryClient) {
				client.WithBaseDenom("", assert.AnError)
				client.WithGetEipBaseFee("", nil)
			},
			expectError: true,
		},
		{
			name: "Error getting EIP base fee",
			setupMocks: func(client *mocks.TxFeesQueryClient) {
				client.WithBaseDenom("wbtc", nil)
				client.WithGetEipBaseFee("", assert.AnError)
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupMocks(&tt.txFeesClient)

			baseFee, err := sqstx.CalculateFeePrice(context.TODO(), &tt.txFeesClient)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCoin, baseFee.Denom)
				assert.Equal(t, tt.expectedBaseFee, baseFee.CurrentFee)
			}
		})
	}
}

func TestCalculateFeeAmount(t *testing.T) {
	tests := []struct {
		name     string
		baseFee  osmomath.Dec
		gas      uint64
		expected osmomath.Int
	}{
		{
			name:     "Zero base fee",
			baseFee:  osmomath.NewDec(0),
			gas:      1000,
			expected: osmomath.NewInt(0),
		},
		{
			name:     "Zero gas",
			baseFee:  osmomath.NewDec(100),
			gas:      0,
			expected: osmomath.NewInt(0),
		},
		{
			name:     "Normal case",
			baseFee:  osmomath.NewDecWithPrec(5, 1), // 0.5
			gas:      100000,
			expected: osmomath.NewInt(50000),
		},
		{
			name:     "Large numbers",
			baseFee:  osmomath.NewDec(1000),
			gas:      1000000,
			expected: osmomath.NewInt(1000000000),
		},
		{
			name:     "Fractional result",
			baseFee:  osmomath.NewDecWithPrec(33, 2), // 0.33
			gas:      10000,
			expected: osmomath.NewInt(3300),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sqstx.CalculateFeeAmount(tt.baseFee, tt.gas)
			assert.True(t, tt.expected.Equal(result), "Expected %s, but got %s", tt.expected, result)
		})
	}
}
