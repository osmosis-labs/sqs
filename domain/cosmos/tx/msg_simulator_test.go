package tx_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	routerrepo "github.com/osmosis-labs/sqs/router/repository"
	"github.com/stretchr/testify/assert"
)

const (
	testChainID = "test-chain"
	testKey     = "6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159"
	testDenom   = "eth"
	testBaseFee = "0.1"
	testGasUsed = uint64(50)
	testAmount  = int64(5)
)

var (
	testAccount = &authtypes.BaseAccount{
		Sequence:      13,
		AccountNumber: 1,
	}
	testMsg    = newMsg("sender", "contract", `{"payload": "hello contract"}`)
	testTxJSON = []byte(`{"body":{"messages":[{"@type":"/cosmwasm.wasm.v1.MsgExecuteContract","sender":"sender","contract":"contract","msg":{"payload":"hello contract"},"funds":[]}],"memo":"","timeout_height":"0","extension_options":[],"non_critical_extension_options":[]},"auth_info":{"signer_infos":[{"public_key":{"@type":"/cosmos.crypto.secp256k1.PubKey","key":"A+9dbfKKCHgfmiV2XUWelqidYzZhHR+KtNMvcSzWjdPQ"},"mode_info":{"single":{"mode":"SIGN_MODE_DIRECT"}},"sequence":"13"}],"fee":{"amount":[{"denom":"eth","amount":"5"}],"gas_limit":"50","payer":"","granter":""},"tip":null},"signatures":["aRlC8F2MnDA50tNNTJUk7zPvH/xc5c3Av+yaGQEiU0l0AXJxUdzOUxWHiC74D9ltvbsk0HzWbb+2uetCjdQdfA=="]}`)

	testBaseFeeDec = osmomath.MustNewDecFromStr(testBaseFee)
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
			account: testAccount,
			chainID: testChainID,
			msgs:    []sdk.Msg{testMsg},
			setupMocks: func(calculator mocks.GetCalculateGasMock) tx.CalculateGasFn {
				return calculator(&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}}, testGasUsed, nil)
			},
			expectedSimulateResponse: &txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}},
			expectedGas:              testGasUsed,
			expectedError:            nil,
		},
		{
			name:    "Simulation error",
			account: &authtypes.BaseAccount{AccountNumber: 2, Sequence: 2},
			chainID: testChainID,
			msgs:    []sdk.Msg{},
			setupMocks: func(calculator mocks.GetCalculateGasMock) tx.CalculateGasFn {
				return calculator(&txtypes.SimulateResponse{}, testGasUsed, assert.AnError)
			},
			expectedSimulateResponse: nil,
			expectedGas:              testGasUsed,
			expectedError:            assert.AnError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calculateGasFnMock := tt.setupMocks(mocks.DefaultGetCalculateGasMock)
			routerRepository := routerrepo.New(&log.NoOpLogger{})
			gasCalculator := tx.NewMsgSimulator(nil, calculateGasFnMock, routerRepository)

			result, gas, err := gasCalculator.SimulateMsgs(
				encodingConfig.TxConfig,
				tt.account,
				tt.chainID,
				tt.msgs,
			)

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
		setupMocks    func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn
		preSetBaseFee domain.BaseFee
		account       *authtypes.BaseAccount
		chainID       string
		msgs          []sdk.Msg
		expectedJSON  []byte
		expectedError bool
	}{
		{
			name: "Valid transaction",
			setupMocks: func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey(testKey)
				return calculator(&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}}, testGasUsed, nil)
			},
			account: testAccount,
			chainID: testChainID,
			msgs:    []sdk.Msg{testMsg},
			preSetBaseFee: domain.BaseFee{
				Denom:      testDenom,
				CurrentFee: testBaseFeeDec,
			},
			expectedJSON:  testTxJSON,
			expectedError: false,
		},
		{
			name: "Error building transaction",
			setupMocks: func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey(testKey)
				return calculator(&txtypes.SimulateResponse{}, testGasUsed, assert.AnError)
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
			keyring := mocks.Keyring{}
			routerRepository := routerrepo.New(&log.NoOpLogger{})
			routerRepository.SetBaseFee(tc.preSetBaseFee)
			calculateGasFnMock := tc.setupMocks(mocks.DefaultGetCalculateGasMock, &keyring)
			msgSimulator := tx.NewMsgSimulator(nil, calculateGasFnMock, routerRepository)

			txBuilder, err := msgSimulator.BuildTx(
				context.Background(),
				&keyring,
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
				assert.Equal(t, string(tc.expectedJSON), string(txJSONBytes))
			}
		})
	}
}

func TestPriceMsgs(t *testing.T) {
	testCases := []struct {
		name            string
		setupMocks      func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn
		account         *authtypes.BaseAccount
		chainID         string
		msgs            []sdk.Msg
		preSetBaseFee   domain.BaseFee
		expectedGas     uint64
		expectedFeeCoin sdk.Coin
		expectedBaseFee osmomath.Dec
		expectedError   bool
	}{
		{
			name: "Valid transaction",
			setupMocks: func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey(testKey)

				return calculator(&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}}, testGasUsed, nil)
			},
			account: testAccount,
			chainID: testChainID,
			msgs:    []sdk.Msg{testMsg},
			preSetBaseFee: domain.BaseFee{
				Denom:      testDenom,
				CurrentFee: testBaseFeeDec,
			},
			expectedGas:     testGasUsed,
			expectedFeeCoin: sdk.Coin{Denom: testDenom, Amount: osmomath.NewInt(testAmount)},
			expectedBaseFee: testBaseFeeDec,
			expectedError:   false,
		},
		{
			name: "Error building transaction",
			setupMocks: func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey(testKey)

				return calculator(&txtypes.SimulateResponse{}, testGasUsed, assert.AnError)
			},
			preSetBaseFee: domain.BaseFee{
				Denom:      testDenom,
				CurrentFee: testBaseFeeDec,
			},
			account: &authtypes.BaseAccount{
				Sequence:      8,
				AccountNumber: 51,
			},
			expectedFeeCoin: sdk.Coin{},
			expectedBaseFee: osmomath.Dec{},
			expectedError:   true,
		},
		{
			name: "Invalid base fee",
			setupMocks: func(calculator mocks.GetCalculateGasMock, keyring *mocks.Keyring) tx.CalculateGasFn {
				keyring.WithGetKey(testKey)

				return calculator(&txtypes.SimulateResponse{GasInfo: &sdk.GasInfo{GasUsed: 100000}}, testGasUsed, nil)
			},
			account:         testAccount,
			chainID:         testChainID,
			msgs:            []sdk.Msg{testMsg},
			expectedGas:     testGasUsed,
			expectedFeeCoin: sdk.Coin{},
			expectedBaseFee: osmomath.Dec{},
			expectedError:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keyring := mocks.Keyring{}
			routerRepository := routerrepo.New(&log.NoOpLogger{})
			routerRepository.SetBaseFee(tc.preSetBaseFee)

			calculateGasFnMock := tc.setupMocks(mocks.DefaultGetCalculateGasMock, &keyring)
			msgSimulator := tx.NewMsgSimulator(nil, calculateGasFnMock, routerRepository)

			priceInfo := msgSimulator.PriceMsgs(
				context.Background(),
				encodingConfig.TxConfig,
				tc.account,
				tc.chainID,
				tc.msgs...,
			)

			if tc.expectedError {
				assert.NotEmpty(t, priceInfo.Err)
				assert.Equal(t, priceInfo.AdjustedGasUsed, uint64(0))
				assert.Equal(t, priceInfo.FeeCoin, sdk.Coin{})
			} else {
				assert.Empty(t, priceInfo.Err)
				assert.Equal(t, tc.expectedGas, priceInfo.AdjustedGasUsed)
				assert.Equal(t, tc.expectedFeeCoin, priceInfo.FeeCoin)
				assert.Equal(t, tc.expectedBaseFee, priceInfo.BaseFee)
			}
		})
	}
}
