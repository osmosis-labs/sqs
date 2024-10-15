package claimbot_test

import (
	"context"
	"testing"

	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestSendBatchClaimTx(t *testing.T) {
	tests := []struct {
		name             string
		contractAddress  string
		claims           orderbookdomain.Orders
		setupMocks       func(*mocks.Keyring, *mocks.AuthQueryClientMock, *mocks.TxFeesQueryClient, *mocks.GasCalculator, *mocks.TxServiceClient)
		setSendTxFunc    func() []byte
		expectedResponse *sdk.TxResponse
		expectedError    bool
	}{
		{
			name:            "AuthQueryClient.GetAccountFunc returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo0address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				authQueryClient.WithGetAccount(nil, assert.AnError)
			},
			expectedResponse: &sdk.TxResponse{},
			expectedError:    true,
		},
		{
			name:            "BuildTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo0address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				authQueryClient.WithGetAccount(&authtypes.QueryAccountResponse{
					Account: authtypes.Account{
						AccountNumber: 3,
						Sequence:      31,
					},
				}, nil)
				gasCalculator.WithCalculateGas(nil, 0, assert.AnError) // Fail BuildTx
			},
			expectedResponse: &sdk.TxResponse{},
			expectedError:    true,
		},
		{
			name:            "SendTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo5address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				gasCalculator.WithCalculateGas(nil, 51, nil)
				txfeesClient.WithBaseDenom("uosmo", nil)
				txfeesClient.WithGetEipBaseFee("0.2", nil)
				authQueryClient.WithGetAccount(&authtypes.QueryAccountResponse{
					Account: authtypes.Account{
						AccountNumber: 83,
						Sequence:      5,
					},
				}, nil)
				txServiceClient.WithBroadcastTx(nil, assert.AnError) // SendTx returns error
			},
			expectedResponse: &sdk.TxResponse{},
			expectedError:    true,
		},
		{
			name:            "Successful transaction",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 1, OrderId: 100},
				{TickId: 2, OrderId: 200},
			},
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo1address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				gasCalculator.WithCalculateGas(nil, 51, nil)
				txfeesClient.WithBaseDenom("uosmo", nil)
				txfeesClient.WithGetEipBaseFee("0.15", nil)
				authQueryClient.WithGetAccount(&authtypes.QueryAccountResponse{
					Account: authtypes.Account{
						AccountNumber: 1,
						Sequence:      1,
					},
				}, nil)

				txServiceClient.BroadcastTxFunc = func(ctx context.Context, in *txtypes.BroadcastTxRequest, opts ...grpc.CallOption) (*txtypes.BroadcastTxResponse, error) {
					return &txtypes.BroadcastTxResponse{
						TxResponse: &sdk.TxResponse{
							Data: string(in.TxBytes), // Assigning the txBytes to response Data to compare it later
						},
					}, nil
				}
			},
			expectedResponse: &sdk.TxResponse{
				Data: "\n\x90\x01\n\x8d\x01\n$/cosmwasm.wasm.v1.MsgExecuteContract\x12e\n\x1fosmo1daek6me3v9jxgun9wdes7m4n5q\x12\x14osmo1contractaddress\x1a,{\"batch_claim\":{\"orders\":[[1,100],[2,200]]}}\x12b\nP\nF\n\x1f/cosmos.crypto.secp256k1.PubKey\x12#\n!\x03\xef]m\xf2\x8a\bx\x1f\x9a%v]E\x9e\x96\xa8\x9dc6a\x1d\x1f\x8a\xb4\xd3/q,֍\xd3\xd0\x12\x04\n\x02\b\x01\x18\x01\x12\x0e\n\n\n\x05uosmo\x12\x018\x103\x1a@Xߠ&\xea\xb8\x0e\xefؓf\xb3\xe7DMӡW\x99h\u008e\xbdh\xef\\\xd3\xd7\x02\xf1\xdc\xe1&\r\x91\xdd\xcdtu\xee\xdeJ\x90\x1a\x7f\xb2(L\x15\xe0+'\xf5\xe3\fV\t3!\xa2,\x802z",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			keyring := mocks.Keyring{}
			authQueryClient := mocks.AuthQueryClientMock{}
			txFeesClient := mocks.TxFeesQueryClient{}
			gasCalculator := mocks.GasCalculator{}
			txServiceClient := mocks.TxServiceClient{}

			tt.setupMocks(&keyring, &authQueryClient, &txFeesClient, &gasCalculator, &txServiceClient)

			response, err := claimbot.SendBatchClaimTx(ctx, &keyring, &authQueryClient, &txFeesClient, &gasCalculator, &txServiceClient, tt.contractAddress, tt.claims)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResponse, response)
			}
		})
	}
}

func TestGetAccount(t *testing.T) {
	newQueryClient := func(resp *authtypes.QueryAccountResponse, err error) authtypes.QueryClient {
		return &mocks.AuthQueryClientMock{
			GetAccountFunc: func(ctx context.Context, address string) (*authtypes.QueryAccountResponse, error) {
				return resp, err
			},
		}
	}
	tests := []struct {
		name           string
		address        string
		queryClient    authtypes.QueryClient
		expectedResult sqstx.Account
		expectedError  bool
	}{
		{
			name:    "Successful account retrieval",
			address: "osmo1f4tvsdukfwh6s9swrc24gkuz23tp8pd3e9r5fa",
			queryClient: newQueryClient(&authtypes.QueryAccountResponse{
				Account: authtypes.Account{
					Sequence:      123,
					AccountNumber: 456,
				},
			}, nil),
			expectedResult: tx.Account{
				Sequence:      123,
				AccountNumber: 456,
			},
			expectedError: false,
		},
		{
			name:           "Error retrieving account",
			address:        "osmo1jllfytsz4dryxhz5tl7u73v29exsf80vz52ucc",
			queryClient:    newQueryClient(nil, assert.AnError),
			expectedResult: tx.Account{},
			expectedError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := claimbot.GetAccount(context.Background(), tt.queryClient, tt.address)
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
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
