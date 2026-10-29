package coingeckopoller

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// newTestServer serves body with status for every request and records the last request's query.
func newTestServer(t *testing.T, status int, header http.Header, body string) (*httptest.Server, *url.Values) {
	t.Helper()
	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		for key, values := range header {
			w.Header()[key] = values
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &query
}

func mustFetch(t *testing.T, rawURL, quoteCurrency string, client *http.Client) fetchFunc {
	t.Helper()
	fetch, err := newHTTPFetch(rawURL, quoteCurrency, client)
	require.NoError(t, err)
	return fetch
}

func TestHTTPFetch_KeepsOnlyUsablePrices(t *testing.T) {
	body := `{
		"atom": {"usd": 4.25},
		"tiny": {"usd": 1.5e-09},
		"a&b": {"usd": 3},
		"zero": {"usd": 0},
		"negative": {"usd": -1},
		"null": {"usd": null},
		"string": {"usd": "1"},
		"other-currency": {"eur": 2},
		"empty": {},
		"malformed": [],
		"unrequested": {"usd": 7}
	}`
	server, query := newTestServer(t, http.StatusOK, nil, body)
	ids := []string{"atom", "tiny", "a&b", "zero", "negative", "null", "string", "other-currency", "empty", "malformed", "missing"}

	// The quote currency is matched in lower case, as Coingecko answers.
	prices, err := mustFetch(t, server.URL, " USD ", server.Client())(context.Background(), ids)
	require.NoError(t, err)
	require.Equal(t, map[string]osmomath.BigDec{
		"atom": osmomath.MustNewBigDecFromStr("4.25"),
		// Full precision is kept.
		"tiny": osmomath.MustNewBigDecFromStr("0.0000000015"),
		"a&b":  osmomath.NewBigDec(3),
	}, prices)
	// The reserved character is escaped, so it does not cut off the ids after it.
	require.Equal(t, strings.Join(ids, ","), query.Get("ids"))
	require.Equal(t, "usd", query.Get("vs_currencies"))
}

func TestHTTPFetch_KeepsQueryParametersOfTheURL(t *testing.T) {
	server, query := newTestServer(t, http.StatusOK, nil, `{"atom": {"usd": 4}}`)

	_, err := mustFetch(t, server.URL+"/simple/price?x_cg_pro_api_key=secret", "usd", server.Client())(context.Background(), []string{"atom"})
	require.NoError(t, err)
	require.Equal(t, "secret", query.Get("x_cg_pro_api_key"))
	require.Equal(t, "atom", query.Get("ids"))
}

func TestHTTPFetch_RateLimited(t *testing.T) {
	for header, want := range map[string]time.Duration{"7": 7 * time.Second, "": 0} {
		server, _ := newTestServer(t, http.StatusTooManyRequests, http.Header{"Retry-After": {header}}, "")

		_, err := mustFetch(t, server.URL, "usd", server.Client())(context.Background(), []string{"atom"})
		var rateLimited *rateLimitError
		require.ErrorAs(t, err, &rateLimited)
		require.Equal(t, want, rateLimited.retryAfter, header)
	}
}

func TestHTTPFetch_FailedRequests(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"server error", http.StatusBadGateway, ""},
		{"not an object", http.StatusOK, `[]`},
		{"null", http.StatusOK, `null`},
		{"truncated body", http.StatusOK, `{"atom": {"usd": 4`},
		{"answers for no requested id", http.StatusOK, `{}`},
		{"error object", http.StatusOK, `{"status": {"error_code": 429, "error_message": "rate limited"}}`},
		{"body over the size limit", http.StatusOK, `{"atom": {"usd": 4}, "pad": "` + strings.Repeat("x", maxResponseBytes) + `"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newTestServer(t, tt.status, nil, tt.body)

			_, err := mustFetch(t, server.URL, "usd", server.Client())(context.Background(), []string{"atom"})
			require.Error(t, err)
			var rateLimited *rateLimitError
			require.False(t, errors.As(err, &rateLimited))
		})
	}
}

func TestNewHTTPFetch_InvalidConfig(t *testing.T) {
	tests := []struct{ name, url, quote string }{
		{"empty URL", "", "usd"},
		{"no scheme", "prices.osmosis.zone/api/v3/simple/price", "usd"},
		{"unparsable URL", "https://prices.osmosis.zone/%zz", "usd"},
		{"empty quote currency", "https://prices.osmosis.zone/api/v3/simple/price", " "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newHTTPFetch(tt.url, tt.quote, http.DefaultClient)
			require.Error(t, err)
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		value string
		want  time.Duration
	}{
		{"", 0},
		{"soon", 0},
		{"12", 12 * time.Second},
		{" 3 ", 3 * time.Second},
		{"-5", 0},
		{"999999999999", maxInterval},
		{now.Add(20 * time.Second).Format(http.TimeFormat), 20 * time.Second},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0},
		{now.Add(time.Hour).Format(http.TimeFormat), maxInterval},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			require.Equal(t, tt.want, parseRetryAfter(tt.value, now))
		})
	}
}
