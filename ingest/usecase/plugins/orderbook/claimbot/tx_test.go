package claimbot_test

import (
	"context"
	"testing"

	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"

	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestSendBatchClaimTx(t *testing.T) {
	tests := []struct {
		name             string
		chainID          string
		contractAddress  string
		claims           orderbookdomain.Orders
		setupMocks       func(*mocks.Keyring, *authtypes.BaseAccount, *mocks.TxFeesQueryClient, *mocks.GasCalculator, *mocks.TxServiceClient)
		setSendTxFunc    func() []byte
		expectedResponse *sdk.TxResponse
		expectedError    bool
	}{
		{
			name:            "BuildTx returns error",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 13, OrderId: 99},
			},
			setupMocks: func(keyringMock *mocks.Keyring, account *authtypes.BaseAccount, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo0address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				account = &authtypes.BaseAccount{
					AccountNumber: 3,
					Sequence:      31,
				}
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
			setupMocks: func(keyringMock *mocks.Keyring, account *authtypes.BaseAccount, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo5address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				gasCalculator.WithCalculateGas(nil, 51, nil)
				txfeesClient.WithBaseDenom("uosmo", nil)
				txfeesClient.WithGetEipBaseFee("0.2", nil)
				account = &authtypes.BaseAccount{
					AccountNumber: 83,
					Sequence:      5,
				}
				txServiceClient.WithBroadcastTx(nil, assert.AnError) // SendTx returns error
			},
			expectedResponse: &sdk.TxResponse{},
			expectedError:    true,
		},
		{
			name:            "Successful transaction",
			chainID:         "osmosis-1",
			contractAddress: "osmo1contractaddress",
			claims: orderbookdomain.Orders{
				{TickId: 1, OrderId: 100},
				{TickId: 2, OrderId: 200},
			},
			setupMocks: func(keyringMock *mocks.Keyring, account *authtypes.BaseAccount, txfeesClient *mocks.TxFeesQueryClient, gasCalculator *mocks.GasCalculator, txServiceClient *mocks.TxServiceClient) {
				keyringMock.WithGetAddress("osmo1address")
				keyringMock.WithGetKey("6cf5103c60c939a5f38e383b52239c5296c968579eec1c68a47d70fbf1d19159")
				gasCalculator.WithCalculateGas(nil, 51, nil)
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
			expectedResponse: &sdk.TxResponse{
				Data: "\n\x90\x01\n\x8d\x01\n$/cosmwasm.wasm.v1.MsgExecuteContract\x12e\n\x1fosmo1daek6me3v9jxgun9wdes7m4n5q\x12\x14osmo1contractaddress\x1a,{\"batch_claim\":{\"orders\":[[1,100],[2,200]]}}\x12`\nN\nF\n\x1f/cosmos.crypto.secp256k1.PubKey\x12#\n!\x03\xef]m\xf2\x8a\bx\x1f\x9a%v]E\x9e\x96\xa8\x9dc6a\x1d\x1f\x8a\xb4\xd3/q,֍\xd3\xd0\x12\x04\n\x02\b\x01\x12\x0e\n\n\n\x05uosmo\x12\x018\x103\x1a@\x1dI\xb5/D\xd0L\v2\xacg\x91\xb3;b+\xdb\xf6\xe0\x1c\x92\xee\xb8d\xc4&%<ڵ\x81\xd6u\xeb-\xf0ੌ\xf5\xa8);\x19\xfc%@\r\xfb2\x05AI\x13\xf3)=\n\xcf~\xb0\"\xf0\xb1",
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
			gasCalculator := mocks.GasCalculator{}
			txServiceClient := mocks.TxServiceClient{}

			tt.setupMocks(&keyring, &account, &txFeesClient, &gasCalculator, &txServiceClient)

			response, err := claimbot.SendBatchClaimTx(ctx, &keyring, &txFeesClient, &gasCalculator, &txServiceClient, tt.chainID, &account, tt.contractAddress, tt.claims)
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
