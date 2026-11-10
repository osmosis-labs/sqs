package http_test

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
	deliveryhttp "github.com/osmosis-labs/sqs/delivery/http"
)

type serializerResponse struct {
	Amount       osmomath.Int      `json:"amount"`
	Price        osmomath.Dec      `json:"price"`
	NilAmount    osmomath.Int      `json:"nil_amount"`
	NilPrice     osmomath.Dec      `json:"nil_price"`
	ZeroPrice    osmomath.Dec      `json:"zero_price"`
	PricePtr     *osmomath.Dec     `json:"price_ptr"`
	Coin         sdk.Coin          `json:"coin"`
	Pools        []uint64          `json:"pools"`
	Labels       map[string]string `json:"labels"`
	Flag         bool              `json:"flag"`
	Count        uint64            `json:"count"`
	FlagOmit     bool              `json:"flag_omit,omitempty"`
	CountOmit    uint64            `json:"count_omit,omitempty"`
	CountOmitted uint64            `json:"count_omitted,omitzero"`
	Note         string            `json:"note,omitempty"`
	Optional     *sdk.Coin         `json:"optional,omitempty"`
}

func serveJSON(t *testing.T, target string, value any) *httptest.ResponseRecorder {
	t.Helper()
	e := deliveryhttp.NewEcho()
	e.GET("/", func(c echo.Context) error { return c.JSON(http.StatusOK, value) })
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestJSONSerializer_Response(t *testing.T) {
	// 2^80 does not fit an int64 or a float64 mantissa: it must stay an exact string.
	amount, ok := osmomath.NewIntFromString("1208925819614629174706176")
	require.True(t, ok)

	rec := serveJSON(t, "/", serializerResponse{
		Amount:    amount,
		Price:     osmomath.MustNewDecFromStr("0.000000000000000001"),
		ZeroPrice: osmomath.ZeroDec(),
		Coin:      sdk.NewCoin("uosmo", osmomath.NewInt(5)),
	})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, echo.MIMEApplicationJSON, rec.Header().Get(echo.HeaderContentType))
	require.True(t, strings.HasSuffix(rec.Body.String(), "}\n"), "body ends with one newline: %q", rec.Body.String())

	// v2 representation: nil slice and map are [] and {}; zero numbers and false booleans are
	// encoded even with omitempty; omitzero, empty strings and nil pointers are omitted.
	// Uninitialized Osmosis numbers keep their methods' "0"; an initialized zero Dec has 18 digits.
	require.JSONEq(t, `{
		"amount": "1208925819614629174706176",
		"price": "0.000000000000000001",
		"nil_amount": "0",
		"nil_price": "0",
		"zero_price": "0.000000000000000000",
		"price_ptr": null,
		"coin": {"denom": "uosmo", "amount": "5"},
		"pools": [],
		"labels": {},
		"flag": false,
		"count": 0,
		"flag_omit": false,
		"count_omit": 0
	}`, rec.Body.String())
}

// Handlers echo user input in error messages. Invalid UTF-8 there is replaced with U+FFFD, as v1
// did, so a bad query parameter still yields its 400 rather than an encoding failure.
func TestJSONSerializer_InvalidUTF8IsReplaced(t *testing.T) {
	e := deliveryhttp.NewEcho()
	e.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusBadRequest, map[string]string{"message": "invalid denom: " + c.QueryParam("base")})
	})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?base=%FF", nil))

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"message":"invalid denom: \ufffd"}`, rec.Body.String())
}

func TestJSONSerializer_PrettyResponse(t *testing.T) {
	rec := serveJSON(t, "/?pretty", map[string]osmomath.Dec{"a": osmomath.OneDec()})

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "{\n  \"a\": \"1.000000000000000000\"\n}\n", rec.Body.String())
}

// An encoding failure must not leave a partial body behind: Echo's error handler then writes
// a complete error response, so the client receives exactly one JSON document.
func TestJSONSerializer_EncodingFailureLeavesResponseUncommitted(t *testing.T) {
	for name, value := range map[string]any{
		"unsupported type":   map[string]any{"ok": 1, "bad": make(chan int)},
		"failure after data": []any{strings.Repeat("y", 64<<10), osmomath.OneDec(), osmomath.OneInt(), func() {}},
	} {
		t.Run(name, func(t *testing.T) {
			e := deliveryhttp.NewEcho()
			c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
			require.Error(t, deliveryhttp.JSONSerializer{}.Serialize(c, value, ""))
			require.False(t, c.Response().Committed)
			require.Zero(t, c.Response().Size)

			rec := serveJSON(t, "/", value)
			require.Equal(t, http.StatusInternalServerError, rec.Code)
			require.JSONEq(t, `{"message":"Internal Server Error"}`, rec.Body.String())
		})
	}
}

type bindRequest struct {
	Name   string       `json:"name"`
	Amount osmomath.Int `json:"amount"`
}

func postJSON(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	e := deliveryhttp.NewEcho()
	e.POST("/", func(c echo.Context) error {
		var req bindRequest
		if err := c.Bind(&req); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, req)
	})
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func TestJSONSerializer_RequestDecoding(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantBody   string
	}{
		{name: "valid", body: `{"name":"osmo","amount":"12345678901234567890123"}`, wantStatus: http.StatusOK, wantBody: `{"name":"osmo","amount":"12345678901234567890123"}`},
		{name: "unknown fields are ignored", body: `{"name":"osmo","amount":"1","extra":true}`, wantStatus: http.StatusOK, wantBody: `{"name":"osmo","amount":"1"}`},
		{name: "field names match case-sensitively", body: `{"Name":"osmo","amount":"1"}`, wantStatus: http.StatusOK, wantBody: `{"name":"","amount":"1"}`},
		{name: "malformed JSON", body: `{"name":`, wantStatus: http.StatusBadRequest},
		{name: "invalid token", body: `{"name":osmo}`, wantStatus: http.StatusBadRequest},
		{name: "duplicate names", body: `{"name":"a","name":"b","amount":"1"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid UTF-8", body: "{\"name\":\"\xff\",\"amount\":\"1\"}", wantStatus: http.StatusBadRequest},
		{name: "trailing junk", body: `{"name":"osmo","amount":"1"}x`, wantStatus: http.StatusBadRequest},
		{name: "second document", body: `{"name":"osmo","amount":"1"} {}`, wantStatus: http.StatusBadRequest},
		{name: "wrong JSON kind", body: `{"name":1,"amount":"1"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid decimal", body: `{"name":"osmo","amount":"1.5"}`, wantStatus: http.StatusBadRequest},
		{name: "oversized body", body: `{"name":"` + strings.Repeat("x", deliveryhttp.MaxJSONRequestBodyBytes) + `"}`, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := postJSON(t, tt.body)
			require.Equal(t, tt.wantStatus, rec.Code, rec.Body.String())
			if tt.wantBody != "" {
				require.JSONEq(t, tt.wantBody, rec.Body.String())
				return
			}
			// Error responses are a single, well-formed JSON object with a message.
			var errBody struct {
				Message string `json:"message"`
			}
			require.NoError(t, json.Unmarshal(bytes.TrimSpace(rec.Body.Bytes()), &errBody))
			require.NotEmpty(t, errBody.Message)
		})
	}
}

func TestWriteEvent(t *testing.T) {
	t.Run("payload is one data line framed as one event", func(t *testing.T) {
		rec := httptest.NewRecorder()
		res := echo.NewResponse(rec, deliveryhttp.NewEcho())

		require.NoError(t, deliveryhttp.WriteEvent(res, map[string]any{"text": "line1\nline2", "pools": []int(nil)}))

		body := rec.Body.String()
		require.True(t, strings.HasPrefix(body, "id: \ndata: "), body)
		require.True(t, strings.HasSuffix(body, "\n\n"), body)
		lines := strings.Split(strings.TrimSuffix(body, "\n\n"), "\n")
		require.Len(t, lines, 2, body)
		payload := strings.TrimPrefix(lines[1], "data: ")
		require.True(t, jsontext.Value(payload).IsValid(), payload)
		require.JSONEq(t, `{"text":"line1\nline2","pools":[]}`, payload)
		require.True(t, rec.Flushed)
	})

	t.Run("encoding failure writes nothing", func(t *testing.T) {
		rec := httptest.NewRecorder()
		res := echo.NewResponse(rec, deliveryhttp.NewEcho())

		require.Error(t, deliveryhttp.WriteEvent(res, map[string]any{"bad": make(chan int)}))
		require.Zero(t, rec.Body.Len())
		require.False(t, rec.Flushed)
	})
}
