package cosmwasmdomain_test

import (
	"context"
	"testing"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/osmosis-labs/osmosis/osmomath"
	osmojson "github.com/osmosis-labs/osmosis/v31/ingest/types/json"
	"github.com/osmosis-labs/osmosis/v31/x/cosmwasmpool/cosmwasm/msg"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
)

// contract answers every smart query with response and records the last query payload.
func contract(response string) (*mocks.WasmClient, *[]byte) {
	var query []byte
	return &mocks.WasmClient{
		SmartContractStateFunc: func(_ context.Context, in *wasmtypes.QuerySmartContractStateRequest, _ ...grpc.CallOption) (*wasmtypes.QuerySmartContractStateResponse, error) {
			query = in.QueryData
			return &wasmtypes.QuerySmartContractStateResponse{Data: []byte(response)}, nil
		},
	}, &query
}

// The contract query payload keeps its meaning under encoding/json/v2: it is the same JSON the
// Osmosis jsoniter wrapper produced, including string-encoded amounts and decimals.
func TestQueryCosmwasmContract_RequestPayload(t *testing.T) {
	request := msg.NewCalcOutAmtGivenInRequest(
		sdk.NewCoin("uosmo", osmomath.NewIntFromUint64(18_446_744_073_709_551_615).MulRaw(1000)),
		"uatom",
		osmomath.MustNewDecFromStr("0.003"),
	)
	client, query := contract(`{"token_out":{"denom":"uatom","amount":"42"}}`)

	var response msg.CalcOutAmtGivenInResponse
	require.NoError(t, cosmwasmdomain.QueryCosmwasmContract(context.Background(), client, "osmo1contract", &request, &response))

	require.JSONEq(t, `{"calc_out_amt_given_in":{
		"token_in":{"denom":"uosmo","amount":"18446744073709551615000"},
		"token_out_denom":"uatom",
		"swap_fee":"0.003000000000000000"
	}}`, string(*query))
	wrapperBz, err := osmojson.Marshal(&request)
	require.NoError(t, err)
	require.JSONEq(t, string(wrapperBz), string(*query))

	require.Equal(t, "uatom", response.TokenOut.Denom)
	require.Equal(t, "42", response.TokenOut.Amount.String())
}

func TestQueryCosmwasmContract_ResponseDecoding(t *testing.T) {
	request := msg.SpotPriceQueryMsg{SpotPrice: msg.SpotPrice{QuoteAssetDenom: "uosmo", BaseAssetDenom: "uatom"}}

	tests := []struct {
		name    string
		data    string
		want    string
		wantErr bool
	}{
		{name: "contract response", data: `{"spot_price":"1.500000000000000000"}`, want: "1.500000000000000000"},
		{name: "unknown fields are ignored", data: `{"spot_price":"2","extra":1}`, want: "2"},
		// Contracts serialize snake_case field names; a differently cased name is not this field.
		{name: "field names match case-sensitively", data: `{"Spot_Price":"2"}`, want: ""},
		{name: "duplicate field", data: `{"spot_price":"1","spot_price":"2"}`, wantErr: true},
		{name: "trailing data", data: `{"spot_price":"1"}{}`, wantErr: true},
		{name: "malformed", data: `{"spot_price":`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, query := contract(tt.data)
			var response msg.SpotPriceQueryMsgResponse
			err := cosmwasmdomain.QueryCosmwasmContract(context.Background(), client, "osmo1contract", &request, &response)
			require.JSONEq(t, `{"spot_price":{"quote_asset_denom":"uosmo","base_asset_denom":"uatom"}}`, string(*query))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, response.SpotPrice)
		})
	}
}
