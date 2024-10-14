package claimbot_test

import (
	"context"
	"testing"

	"github.com/osmosis-labs/sqs/delivery/grpc"
	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
	"github.com/osmosis-labs/sqs/domain/cosmos/tx"
	sqstx "github.com/osmosis-labs/sqs/domain/cosmos/tx"
	"github.com/osmosis-labs/sqs/domain/keyring"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"

	"github.com/osmosis-labs/osmosis/v26/app/params"

	cosmosClient "github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/stretchr/testify/assert"
)

func TestSendBatchClaimTx(t *testing.T) {
	keyringWithGetAddressFunc := func(mock *mocks.Keyring, address string) {
		mock.GetAddressFunc = func() sdk.AccAddress {
			return sdk.AccAddress(address)
		}
	}

	keyringWithGetKeyFunc := func(mock *mocks.Keyring, key string) {
		mock.GetKeyFunc = func() secp256k1.PrivKey {
			return mock.GenPrivKey(key)
		}
	}

	authQueryClientWithGetAccountFunc := func(mock *mocks.AuthQueryClientMock, response *authtypes.QueryAccountResponse, err error) {
		mock.GetAccountFunc = func(ctx context.Context, address string) (*authtypes.QueryAccountResponse, error) {
			return response, err
		}
	}

	tests := []struct {
		name             string
		contractAddress  string
		claims           orderbookdomain.Orders
		setupMocks       func(*mocks.Keyring, *mocks.AuthQueryClientMock)
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
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock) {
				keyringWithGetAddressFunc(keyringMock, "osmo0address")
				keyringWithGetKeyFunc(keyringMock, "6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				authQueryClientWithGetAccountFunc(authQueryClient, nil, assert.AnError)
			},
			expectedResponse: &sdk.TxResponse{},
			expectedError:    true,
		},
		{
			name:            "SetBuildTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock) {
				keyringWithGetAddressFunc(keyringMock, "osmo0address")
				keyringWithGetKeyFunc(keyringMock, "6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				authQueryClientWithGetAccountFunc(authQueryClient, &authtypes.QueryAccountResponse{
					Account: authtypes.Account{
						AccountNumber: 3,
						Sequence:      31,
					},
				}, nil)

				claimbot.SetBuildTx(func(
					ctx context.Context,
					grpcClient *grpc.Client,
					keyring keyring.Keyring,
					encodingConfig params.EncodingConfig,
					account sqstx.Account,
					chainID string,
					msg ...sdk.Msg,
				) (cosmosClient.TxBuilder, error) {
					return nil, assert.AnError
				})
			},
			expectedResponse: &sdk.TxResponse{},
			expectedError:    true,
		},
		{
			name:            "SetSendTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock) {
				keyringWithGetAddressFunc(keyringMock, "osmo0address")
				keyringWithGetKeyFunc(keyringMock, "6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				authQueryClientWithGetAccountFunc(authQueryClient, &authtypes.QueryAccountResponse{
					Account: authtypes.Account{
						AccountNumber: 3,
						Sequence:      31,
					},
				}, nil)

				claimbot.SetBuildTx(func(
					ctx context.Context,
					grpcClient *grpc.Client,
					keyring keyring.Keyring,
					encodingConfig params.EncodingConfig,
					account sqstx.Account,
					chainID string,
					msg ...sdk.Msg,
				) (cosmosClient.TxBuilder, error) {
					builder := encodingConfig.TxConfig.NewTxBuilder()
					builder.SetMsgs(msg...)
					return builder, nil
				})

				claimbot.SetSendTx(func(ctx context.Context, grpcClient *grpc.Client, txBytes []byte) (*sdk.TxResponse, error) {
					return nil, assert.AnError
				})
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
			setupMocks: func(keyringMock *mocks.Keyring, authQueryClient *mocks.AuthQueryClientMock) {
				keyringWithGetAddressFunc(keyringMock, "osmo1address")
				keyringWithGetKeyFunc(keyringMock, "6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				authQueryClientWithGetAccountFunc(authQueryClient, &authtypes.QueryAccountResponse{
					Account: authtypes.Account{
						AccountNumber: 1,
						Sequence:      1,
					},
				}, nil)

				claimbot.SetBuildTx(func(
					ctx context.Context,
					grpcClient *grpc.Client,
					keyring keyring.Keyring,
					encodingConfig params.EncodingConfig,
					account sqstx.Account,
					chainID string,
					msg ...sdk.Msg,
				) (cosmosClient.TxBuilder, error) {
					builder := encodingConfig.TxConfig.NewTxBuilder()
					builder.SetMsgs(msg...)
					return builder, nil
				})

				claimbot.SetSendTx(func(ctx context.Context, grpcClient *grpc.Client, txBytes []byte) (*sdk.TxResponse, error) {
					return &sdk.TxResponse{
						Data: string(txBytes), // Assigning the txBytes to response Data to compare it later
					}, nil
				})
			},
			expectedResponse: &sdk.TxResponse{
				Data: "\n\x90\x01\n\x8d\x01\n$/cosmwasm.wasm.v1.MsgExecuteContract\x12e\n\x1fosmo1daek6me3v9jxgun9wdes7m4n5q\x12\x14osmo1contractaddress\x1a,{\"batch_claim\":{\"orders\":[[1,100],[2,200]]}}\x12\x02\x12\x00",
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			keyring := mocks.Keyring{}
			authQueryClient := mocks.AuthQueryClientMock{}

			tt.setupMocks(&keyring, &authQueryClient)

			response, err := claimbot.SendBatchClaimTx(ctx, &keyring, nil, &authQueryClient, tt.contractAddress, tt.claims)
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
