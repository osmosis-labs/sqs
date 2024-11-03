package tx_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/stretchr/testify/assert"
)

func TestSimulateMsgs(t *testing.T) {
	tests := []struct {
		name                     string
		account                  *authtypes.BaseAccount
		chainID                  string
		msgs                     []sdk.Msg
		setupMocks               func(calculator mocks.GetCalculateGasMock) tx.CalculateGasFn
		expectedSimulateResponse *txtypes.SimulateResponse
		expectedGas              uint64
		expectedError            error
	}{
		{
			name:    "Successful simulation",
			account: &authtypes.BaseAccount{AccountNumber: 1, Sequence: 1},
			chainID: "test-chain",
			msgs:    []sdk.Msg{newMsg("sender", "contract", `{}`)},
			setupMocks: func(calculator mocks.GetCalculateGasMock) tx.CalculateGasFn {
				return calculator(&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}}, 50, nil)
			},
			expectedSimulateResponse: &txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}},
			expectedGas:              50,
			expectedError:            nil,
		},
		{
			name:    "Simulation error",
			account: &authtypes.BaseAccount{AccountNumber: 2, Sequence: 2},
			chainID: "test-chain",
			msgs:    []sdk.Msg{},
			setupMocks: func(calculator mocks.GetCalculateGasMock) tx.CalculateGasFn {
				return calculator(&txtypes.SimulateResponse{}, 3, assert.AnError)
			},
			expectedSimulateResponse: nil,
			expectedGas:              3,
			expectedError:            assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup the mock
			calculateGasFnMock := tt.setupMocks(mocks.DefaultGetCalculateGasMock)

			// Create the gas calculator
			gasCalculator := tx.NewGasCalculator(nil, calculateGasFnMock)

			// Call the function
			result, gas, err := gasCalculator.SimulateMsgs(
				encodingConfig.TxConfig,
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

func TestBuildTx(t *testing.T) {
	testCases := []struct {
		name          string
		setupMocks    func(calculator mocks.GetCalculateGasMock, txFeesClient *mocks.TxFeesQueryClient, keyring *mocks.Keyring) tx.CalculateGasFn
		account       *authtypes.BaseAccount
		chainID       string
		msgs          []sdk.Msg
		expectedJSON  []byte
		expectedError bool
	}{
		{
			name: "Valid transaction",
			setupMocks: func(calculator mocks.GetCalculateGasMock, txFeesClient *mocks.TxFeesQueryClient, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				txFeesClient.WithBaseDenom("eth", nil)
				txFeesClient.WithGetEipBaseFee("0.1", nil)

				return calculator(&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}}, 50, nil)
			},
			account: &authtypes.BaseAccount{
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
			setupMocks: func(calculator mocks.GetCalculateGasMock, txFeesClient *mocks.TxFeesQueryClient, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")

				return calculator(&txtypes.SimulateResponse{}, 50, assert.AnError)
			},
			account: &authtypes.BaseAccount{
				Sequence:      8,
				AccountNumber: 51,
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			txFeesClient := mocks.TxFeesQueryClient{}
			keyring := mocks.Keyring{}

			// Setup the mock
			calculateGasFnMock := tc.setupMocks(mocks.DefaultGetCalculateGasMock, &txFeesClient, &keyring)

			// Create the gas calculator
			msgSimulator := tx.NewGasCalculator(nil, calculateGasFnMock)

			txBuilder, err := msgSimulator.BuildTx(
				context.Background(),
				&keyring,
				&txFeesClient,
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
