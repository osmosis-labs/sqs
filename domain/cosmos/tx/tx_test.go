package tx_test

import (
	"context"
	"testing"

	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mocks"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v26/app"
	txfeestypes "github.com/osmosis-labs/osmosis/v26/x/txfees/types"

	txclient "github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"

	gogogrpc "github.com/cosmos/gogoproto/grpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

var (
	encodingConfig = app.MakeEncodingConfig()

	newQueryBaseDenomResponse = func(denom string) *txfeestypes.QueryBaseDenomResponse {
		return &txfeestypes.QueryBaseDenomResponse{BaseDenom: denom}
	}

	newQueryEipBaseFeeResponse = func(baseFee string) *txfeestypes.QueryEipBaseFeeResponse {
		return &txfeestypes.QueryEipBaseFeeResponse{
			BaseFee: osmomath.MustNewDecFromStr(baseFee),
		}
	}

	calculateGasFunc = func(response *txtypes.SimulateResponse, n uint64, err error) sqstx.CalculateGasFunc {
		return func(clientCtx gogogrpc.ClientConn, txf txclient.Factory, msgs ...sdk.Msg) (*txtypes.SimulateResponse, uint64, error) {
			return response, n, err
		}
	}

	newMsg = func(sender, contract, msg string) sdk.Msg {
		return &wasmtypes.MsgExecuteContract{
			Sender:   sender,
			Contract: contract,
			Msg:      []byte(msg),
			Funds:    sdk.NewCoins(),
		}
	}
)

func TestBuildTx(t *testing.T) {
	testCases := []struct {
		name          string
		keyring       keyring.Keyring
		calculateGas  sqstx.CalculateGasFunc
		txFeesClient  mocks.TxFeesQueryClient
		account       sqstx.Account
		chainID       string
		msgs          []sdk.Msg
		expectedJSON  []byte
		expectedError bool
	}{
		{
			name: "Valid transaction",
			keyring: &mocks.Keyring{
				GetKeyFunc: func() secp256k1.PrivKey {
					return (&mocks.Keyring{}).GenPrivKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				},
			},
			calculateGas: calculateGasFunc(nil, 50, nil),
			txFeesClient: mocks.TxFeesQueryClient{
				BaseDenomFunc: func(ctx context.Context, in *txfeestypes.QueryBaseDenomRequest, opts ...grpc.CallOption) (*txfeestypes.QueryBaseDenomResponse, error) {
					return newQueryBaseDenomResponse("eth"), nil
				},
				GetEipBaseFeeFunc: func(ctx context.Context, in *txfeestypes.QueryEipBaseFeeRequest, opts ...grpc.CallOption) (*txfeestypes.QueryEipBaseFeeResponse, error) {
					return newQueryEipBaseFeeResponse("0.1"), nil
				},
			},
			account: sqstx.Account{
				Sequence:      13,
				AccountNumber: 1,
			},
			chainID:       "test-chain",
			msgs:          []sdk.Msg{newMsg("sender", "contract", `{"payload": "hello contract"}`)},
			expectedJSON:  []byte(`{"body":{"messages":[{"@type":"/cosmwasm.wasm.v1.MsgExecuteContract","sender":"sender","contract":"contract","msg":{"payload":"hello contract"},"funds":[]}],"memo":"","timeout_height":"0","extension_options":[],"non_critical_extension_options":[]},"auth_info":{"signer_infos":[{"public_key":{"@type":"/cosmos.crypto.secp256k1.PubKey","key":"A+9dbfKKCHgfmiV2XUWelqidYzZhHR+KtNMvcSzWjdPQ"},"mode_info":{"single":{"mode":"SIGN_MODE_DIRECT"}},"sequence":"13"}],"fee":{"amount":[{"denom":"eth","amount":"5"}],"gas_limit":"50","payer":"","granter":""},"tip":null},"signatures":["aRlC8F2MnDA50tNNTJUk7zPvH/xc5c3Av+yaGQEiU0l0AXJxUdzOUxWHiC74D9ltvbsk0HzWbb+2uetCjdQdfA=="]}`),
			expectedError: false,
		},
		{
			name: "Error building transaction",
			keyring: &mocks.Keyring{
				GetKeyFunc: func() secp256k1.PrivKey {
					return (&mocks.Keyring{}).GenPrivKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				},
			},
			calculateGas:  calculateGasFunc(nil, 50, assert.AnError),
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sqstx.SetCalculateGasFunc(tc.calculateGas)
			sqstx.SetTxFeesClient(&tc.txFeesClient)

			txBuilder, err := sqstx.BuildTx(
				context.Background(),
				nil,
				tc.keyring,
				encodingConfig,
				tc.account,
				tc.chainID,
				tc.msgs...,
			)

			if tc.expectedError {
				assert.Error(t, err)
				assert.Nil(t, txBuilder)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, txBuilder)

				txJSONBytes, err := encodingConfig.TxConfig.TxJSONEncoder()(txBuilder.GetTx())
				assert.NoError(t, err)

				// Add more specific assertions here based on the expected output
				assert.Equal(t, string(tc.expectedJSON), string(txJSONBytes))
			}
		})
	}
}

func TestSendTx(t *testing.T) {
	newBroadcastTxFunc := func(txResponse *txtypes.BroadcastTxResponse, err error) func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
		return func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
			return txResponse, err
		}
	}
	tests := []struct {
		name            string
		txBytes         []byte
		txServiceClient mocks.ServiceClient
		expectedResult  *sdk.TxResponse
		expectedError   error
	}{
		{
			name:    "Successful transaction",
			txBytes: []byte("txbytes"),
			txServiceClient: mocks.ServiceClient{
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
			txServiceClient: mocks.ServiceClient{
				BroadcastTxFunc: newBroadcastTxFunc(nil, assert.AnError),
			},
			expectedResult: nil,
			expectedError:  assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqstx.SetTxServiceClient(&tt.txServiceClient)

			result, err := sqstx.SendTx(context.Background(), nil, tt.txBytes)

			assert.Equal(t, tt.expectedResult, result)
			assert.Equal(t, tt.expectedError, err)
		})
	}
}

func TestSimulateMsgs(t *testing.T) {
	tests := []struct {
		name                     string
		account                  sqstx.Account
		chainID                  string
		msgs                     []sdk.Msg
		calculateGas             sqstx.CalculateGasFunc
		expectedSimulateResponse *txtypes.SimulateResponse
		expectedGas              uint64
		expectedError            error
	}{
		{
			name:    "Successful simulation",
			account: sqstx.Account{AccountNumber: 1, Sequence: 1},
			chainID: "test-chain",
			msgs:    []sdk.Msg{newMsg("sender", "contract", `{}`)},
			calculateGas: calculateGasFunc(
				&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}},
				50,
				nil,
			),
			expectedSimulateResponse: &txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}},
			expectedGas:              50,
			expectedError:            nil,
		},
		{
			name:                     "Simulation error",
			account:                  sqstx.Account{AccountNumber: 2, Sequence: 2},
			chainID:                  "test-chain",
			msgs:                     []sdk.Msg{},
			calculateGas:             calculateGasFunc(nil, 3, assert.AnError),
			expectedSimulateResponse: nil,
			expectedGas:              3,
			expectedError:            assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqstx.SetCalculateGasFunc(tt.calculateGas)

			// Call the function
			result, gas, err := sqstx.SimulateMsgs(
				nil,
				encodingConfig,
				tt.account,
				tt.chainID,
				tt.msgs,
			)

			// Assert the results
			assert.Equal(t, tt.expectedSimulateResponse, result)
			assert.Equal(t, tt.expectedGas, gas)
			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
			}
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
		name           string
		gas            uint64
		txFeesClient   mocks.TxFeesQueryClient
		expectedCoin   string
		expectedAmount osmomath.Int
		expectError    bool
	}{
		{
			name: "Normal case",
			gas:  100000,
			txFeesClient: mocks.TxFeesQueryClient{
				BaseDenomFunc: func(ctx context.Context, in *txfeestypes.QueryBaseDenomRequest, opts ...grpc.CallOption) (*txfeestypes.QueryBaseDenomResponse, error) {
					return newQueryBaseDenomResponse("uosmo"), nil
				},
				GetEipBaseFeeFunc: func(ctx context.Context, in *txfeestypes.QueryEipBaseFeeRequest, opts ...grpc.CallOption) (*txfeestypes.QueryEipBaseFeeResponse, error) {
					return newQueryEipBaseFeeResponse("0.5"), nil
				},
			},
			expectedCoin:   "uosmo",
			expectedAmount: osmomath.NewInt(50000),
			expectError:    false,
		},
		{
			name: "Error getting base denom",
			txFeesClient: mocks.TxFeesQueryClient{
				BaseDenomFunc: func(ctx context.Context, in *txfeestypes.QueryBaseDenomRequest, opts ...grpc.CallOption) (*txfeestypes.QueryBaseDenomResponse, error) {
					return nil, assert.AnError
				},
				GetEipBaseFeeFunc: func(ctx context.Context, in *txfeestypes.QueryEipBaseFeeRequest, opts ...grpc.CallOption) (*txfeestypes.QueryEipBaseFeeResponse, error) {
					return nil, nil
				},
			},
			expectError: true,
		},
		{
			name: "Error getting EIP base fee",
			txFeesClient: mocks.TxFeesQueryClient{
				BaseDenomFunc: func(ctx context.Context, in *txfeestypes.QueryBaseDenomRequest, opts ...grpc.CallOption) (*txfeestypes.QueryBaseDenomResponse, error) {
					return newQueryBaseDenomResponse("wbtc"), nil
				},
				GetEipBaseFeeFunc: func(ctx context.Context, in *txfeestypes.QueryEipBaseFeeRequest, opts ...grpc.CallOption) (*txfeestypes.QueryEipBaseFeeResponse, error) {
					return nil, assert.AnError
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqstx.SetTxFeesClient(&tt.txFeesClient)
			result, err := sqstx.CalculateFeeCoin(context.TODO(), nil, tt.gas)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, types.NewCoin(tt.expectedCoin, tt.expectedAmount), result)
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
