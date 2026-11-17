package usecase_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/router/usecase"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/router/usecase/route"
)

// benchmarkQuote is a prepared quote shaped like a large real response: 4 split routes of
// 3 hops each, every pool reduced to its result form, plus token metadata and price info.
func benchmarkQuote() *usecase.QuoteExactAmountIn {
	const (
		routes = 4
		hops   = 3
	)
	amount, _ := osmomath.NewIntFromString("123456789012345678901234")
	fee := osmomath.MustNewDecFromStr("0.002")

	quote := &usecase.QuoteExactAmountIn{
		AmountIn:                sdk.NewCoin("ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2", amount),
		AmountOut:               amount.MulRaw(3),
		LiquidityCap:            amount,
		EffectiveFee:            fee,
		PriceImpact:             osmomath.MustNewDecFromStr("-0.000123456789012345"),
		InBaseOutQuoteSpotPrice: osmomath.MustNewDecFromStr("12.345678901234567890"),
		PriceInfo:               &domain.TxFeeInfo{AdjustedGasUsed: 450_000, FeeCoin: sdk.NewCoin("uosmo", osmomath.NewInt(11_250)), BaseFee: fee},
	}
	for r := range routes {
		routePools := make([]domain.RoutablePool, 0, hops)
		for h := range hops {
			poolType, codeID := poolmanagertypes.Concentrated, uint64(0)
			if h == hops-1 {
				poolType, codeID = poolmanagertypes.CosmWasm, 814
			}
			denom := fmt.Sprintf("factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/token%d", h)
			routePools = append(routePools, pools.NewRoutableResultPool(uint64(1000+r*hops+h), poolType, fee, denom, fee, amount, codeID))
		}
		quote.Route = append(quote.Route, &route.RouteWithOutAmount{
			RouteImpl: route.RouteImpl{Pools: routePools},
			InAmount:  amount.QuoRaw(routes),
			OutAmount: amount.MulRaw(3).QuoRaw(routes),
		})
		quote.Tokens = append(quote.Tokens, usecase.Token{Denom: fmt.Sprintf("token%d", r), LiquidityCapitalization: amount})
	}
	return quote
}

// plainV2Serializer is deliveryhttp.JSONSerializer without osmomath.JSONMarshalOptions: the
// numeric types go through their MarshalJSON methods. It is the baseline the option is measured
// and checked against. Like the production pool, buf is reused across calls (not concurrently).
type plainV2Serializer struct {
	echo.DefaultJSONSerializer // Deserialize, unused here
	buf                        *bytes.Buffer
}

func (s plainV2Serializer) Serialize(c echo.Context, i any, indent string) error {
	var opts []json.Options
	if indent != "" {
		opts = append(opts, jsontext.WithIndent(indent))
	}
	buf := s.buf
	buf.Reset()
	if err := json.MarshalWrite(buf, i, opts...); err != nil {
		return err
	}
	buf.WriteByte('\n')
	_, err := c.Response().Write(buf.Bytes())
	return err
}

func serializeQuote(t testing.TB, serializer echo.JSONSerializer, indent string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(httptest.NewRequest(http.MethodGet, "/router/quote", nil), rec)
	if err := serializer.Serialize(c, benchmarkQuote(), indent); err != nil {
		t.Fatal(err)
	}
	return rec.Body.String()
}

// The production serializer's numeric option must not change a quote's bytes.
func TestQuoteResponseSerialization_MatchesPlainV2(t *testing.T) {
	for _, indent := range []string{"", "  "} {
		want := serializeQuote(t, plainV2Serializer{buf: new(bytes.Buffer)}, indent)
		if got := serializeQuote(t, deliveryhttp.JSONSerializer{}, indent); got != want {
			t.Fatalf("indent %q:\nwant %s\ngot  %s", indent, want, got)
		}
	}
}

// BenchmarkQuoteResponseSerialization compares Echo's default encoding/json serializer (the
// original production codec), plain encoding/json/v2, and the production v2 serializer with
// osmomath.JSONMarshalOptions on the same quote.
func BenchmarkQuoteResponseSerialization(b *testing.B) {
	quote := benchmarkQuote()
	for _, bc := range []struct {
		name       string
		serializer echo.JSONSerializer
	}{
		{name: "v1-echo-default", serializer: echo.DefaultJSONSerializer{}},
		{name: "v2-plain", serializer: plainV2Serializer{buf: new(bytes.Buffer)}},
		{name: "v2", serializer: deliveryhttp.JSONSerializer{}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/router/quote", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			b.ReportAllocs()
			for b.Loop() {
				rec.Body.Reset()
				c.Reset(req, rec)
				if err := bc.serializer.Serialize(c, quote, ""); err != nil {
					b.Fatal(err)
				}
			}
			b.SetBytes(int64(rec.Body.Len()))
		})
	}
}
