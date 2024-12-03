package claimbot_test

import (
	"context"
	"testing"

	cosmosclient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/osmosis-labs/osmosis/v28/app"
	"github.com/osmosis-labs/osmosis/v28/app/params"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestSendBatchClaimTx(t *testing.T) {
	const mockedTxBytes = "mocked-tx-bytes"

	tests := []struct {
		name            string
		chainID         string
		contractAddress string
		claims          orderbookdomain.Orders
		setupMocks      func(*mocks.Keyring, *authtypes.BaseAccount, *mocks.TxFeesQueryClient, *mocks.MsgSimulatorMock, *mocks.TxServiceClient)
		setSendTxFunc   func() []byte

		getEncodingConfigFn func() params.EncodingConfig

		expectedResponse *sdk.TxResponse
		expectedError    bool
	}{
		{
			name:            "BuildTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, account *authtypes.BaseAccount, txfeesClient *mocks.TxFeesQueryClient, msgSimulator *mocks.MsgSimulatorMock, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo0address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				account = &authtypes.BaseAccount{
					AccountNumber: 3,
					Sequence:      31,
				}
				// Fail BuildTx
				msgSimulator.BuildTxFn = func(
					ctx context.Context,
					keyring keyring.Keyring,
					encodingConfig params.EncodingConfig,
					account *authtypes.BaseAccount,
					chainID string,
					msg ...sdk.Msg,
				) (cosmosclient.TxBuilder, error) {
					return nil, assert.AnError
				}
			},
			getEncodingConfigFn: claimbot.DefaultEncodingConfigFn,
			expectedResponse:    &sdk.TxResponse{},
			expectedError:       true,
		},
		{
			name:            "SendTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, account *authtypes.BaseAccount, txfeesClient *mocks.TxFeesQueryClient, msgSimulator *mocks.MsgSimulatorMock, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo5address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				msgSimulator.BuildTxFn = func(
					ctx context.Context,
					keyring keyring.Keyring,
					encodingConfig params.EncodingConfig,
					account *authtypes.BaseAccount,
					chainID string,
					msg ...sdk.Msg,
				) (cosmosclient.TxBuilder, error) {
					return &mocks.TxBuilderMock{
						GetTxFn: func() signing.Tx {
							return &mocks.TxMock{}
						},
					}, nil
				}
				txfeesClient.WithBaseDenom("uosmo", nil)
				txfeesClient.WithGetEipBaseFee("0.2", nil)
				account = &authtypes.BaseAccount{
					AccountNumber: 83,
					Sequence:      5,
				}
				txServiceClient.WithBroadcastTx(nil, assert.AnError) // SendTx returns error
			},
			getEncodingConfigFn: claimbot.DefaultEncodingConfigFn,
			expectedResponse:    &sdk.TxResponse{},
			expectedError:       true,
		},
		{
			name:            "Successful transaction",
			chainID:         "osmosis-1",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 1, OrderId: 100},
				{TickId: 2, OrderId: 200},
			},
			setupMocks: func(keyringMock *mocks.Keyring, account *authtypes.BaseAccount, txfeesClient *mocks.TxFeesQueryClient, msgSimulator *mocks.MsgSimulatorMock, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo1address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				msgSimulator.BuildTxFn = func(
					ctx context.Context,
					keyring keyring.Keyring,
					encodingConfig params.EncodingConfig,
					account *authtypes.BaseAccount,
					chainID string,
					msg ...sdk.Msg,
				) (cosmosclient.TxBuilder, error) {
					return &mocks.TxBuilderMock{
						GetTxFn: func() signing.Tx {
							return &mocks.TxMock{}
						},
					}, nil
				}
				txfeesClient.WithBaseDenom("uosmo", nil)
				txfeesClient.WithGetEipBaseFee("0.15", nil)
				account = &authtypes.BaseAccount{
					AccountNumber: 1,
					Sequence:      1,
				}

				txServiceClient.BroadcastTxFunc = func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
					return &txtypes.BroadcastTxResponse{
						TxResponse: &sdk.TxResponse{
							Data: string(in.TxBytes), // Assigning the txBytes to response Data to compare it later
						},
					}, nil
				}
			},

			getEncodingConfigFn: func() params.EncodingConfig {
				encoding := app.MakeEncodingConfig()
				encoding.TxConfig = &mocks.TxConfigMock{
					TxEncoderFn: func() types.TxEncoder {
						return func(tx types.Tx) ([]byte, error) {
							return []byte(mockedTxBytes), nil
						}
					},
				}
				return encoding
			},

			expectedResponse: &sdk.TxResponse{
				Data: string(mockedTxBytes),
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			keyring := mocks.Keyring{}
			account := authtypes.BaseAccount{}
			txFeesClient := mocks.TxFeesQueryClient{}
			txServiceClient := mocks.TxServiceClient{}

			txSimulatorMock := mocks.MsgSimulatorMock{}

			tt.setupMocks(&keyring, &account, &txFeesClient, &txSimulatorMock, &txServiceClient)

			response, err := claimbot.SendBatchClaimTxInternal(ctx, &keyring, &txFeesClient, &txSimulatorMock, &txServiceClient, tt.chainID, &account, tt.contractAddress, tt.claims, tt.getEncodingConfigFn)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResponse, response)
			}
		})
	}
}

func TestPrepareBatchClaimMsg(t *testing.T) {
	tests := []struct {
		name   string
		claims orderbookdomain.Orders
		want   []byte
	}{
		{
			name: "Single claim",
			claims: orderbookdomain.Orders{
				{TickId: 1, OrderId: 100},
			},
			want: []byte(`{"batch_claim":{"orders":[[1,100]]}}`),
		},
		{
			name: "Multiple claims",
			claims: orderbookdomain.Orders{
				{TickId: 1, OrderId: 100},
				{TickId: 2, OrderId: 200},
				{TickId: 3, OrderId: 300},
			},
			want: []byte(`{"batch_claim":{"orders":[[1,100],[2,200],[3,300]]}}`),
		},
		{
			name:   "Empty claims",
			claims: orderbookdomain.Orders{},
			want:   []byte(`{"batch_claim":{"orders":[]}}`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := claimbot.PrepareBatchClaimMsg(tt.claims)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
