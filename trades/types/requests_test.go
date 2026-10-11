package types_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	// Sets the osmo bech32 prefix.
	_ "github.com/osmosis-labs/osmosis/v31/app/params"

	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/trades/types"
)

func newContext(params url.Values) echo.Context {
	req := httptest.NewRequest(http.MethodGet, "/trades/list?"+params.Encode(), nil)
	return echo.New().NewContext(req, httptest.NewRecorder())
}

func TestRequestValidation(t *testing.T) {
	validCursor := domain.TradeKey{BlockHeight: 72275545, TxIndex: 2, EventIndex: 39}.EncodeCursor()

	tests := []struct {
		name    string
		params  url.Values
		wantErr bool
	}{
		{name: "pool scope", params: url.Values{"poolId": {"1933"}}},
		{name: "ibc denom scope", params: url.Values{"coinMinimalDenom": {"ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"}}},
		{name: "factory alloy denom", params: url.Values{"coinMinimalDenom": {"factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"}}},
		{name: "wallet, limit and cursor", params: url.Values{"poolId": {"1"}, "wallet": {"osmo1l3et2hda5px5r60h8xe5cs49ettdvm9sdn2m6d"}, "limit": {"100"}, "cursor": {validCursor}}},
		{name: "missing scope", params: url.Values{"wallet": {"osmo1l3et2hda5px5r60h8xe5cs49ettdvm9sdn2m6d"}}, wantErr: true},
		{name: "zero pool", params: url.Values{"poolId": {"0"}}, wantErr: true},
		{name: "negative pool", params: url.Values{"poolId": {"-1"}}, wantErr: true},
		{name: "symbol is not a denom", params: url.Values{"coinMinimalDenom": {"U$DC"}}, wantErr: true},
		{name: "bad bech32 wallet", params: url.Values{"poolId": {"1"}, "wallet": {"osmo1notanaddress"}}, wantErr: true},
		{name: "limit zero", params: url.Values{"poolId": {"1"}, "limit": {"0"}}, wantErr: true},
		{name: "limit too large", params: url.Values{"poolId": {"1"}, "limit": {"101"}}, wantErr: true},
		{name: "cursor not base64", params: url.Values{"poolId": {"1"}, "cursor": {"!!"}}, wantErr: true},
		{name: "cursor two-tuple", params: url.Values{"poolId": {"1"}, "cursor": {"MTAuMg"}}, wantErr: true}, // "10.2"
		{name: "cursor zero height", params: url.Values{"poolId": {"1"}, "cursor": {domain.TradeKey{}.EncodeCursor()}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req types.ListTradesRequest
			err := deliveryhttp.ParseRequest(newContext(tc.params), &req)
			require.Equal(t, tc.wantErr, err != nil, "err: %v", err)
		})
	}

	t.Run("list defaults and cursor round trip", func(t *testing.T) {
		var req types.ListTradesRequest
		require.NoError(t, deliveryhttp.ParseRequest(newContext(url.Values{"poolId": {"1"}, "cursor": {validCursor}}), &req))
		require.Equal(t, types.DefaultListLimit, req.Limit)
		require.Equal(t, domain.TradeKey{BlockHeight: 72275545, TxIndex: 2, EventIndex: 39}, *req.Cursor)
	})

	windowTests := []struct {
		windows string
		want    []domain.TradeWindow
		wantErr bool
	}{
		{windows: "", want: domain.TradeWindows},
		{windows: "7d", want: []domain.TradeWindow{domain.TradeWindow7d}},
		{windows: "24h,7d,24h", want: []domain.TradeWindow{domain.TradeWindow24h, domain.TradeWindow7d}},
		{windows: "1h", wantErr: true},
		{windows: "24h,", wantErr: true},
	}
	for _, tc := range windowTests {
		t.Run("windows="+tc.windows, func(t *testing.T) {
			params := url.Values{"coinMinimalDenom": {"uosmo"}}
			if tc.windows != "" {
				params.Set("windows", tc.windows)
			}
			var req types.AggregatesRequest
			err := deliveryhttp.ParseRequest(newContext(params), &req)
			require.Equal(t, tc.wantErr, err != nil, "err: %v", err)
			if !tc.wantErr {
				require.Equal(t, tc.want, req.Windows)
			}
		})
	}
}
