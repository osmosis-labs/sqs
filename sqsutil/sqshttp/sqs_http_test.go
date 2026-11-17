package sqshttp_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sqspassthroughdomain "github.com/osmosis-labs/osmosis/v31/ingest/types/passthroughdomain"
	"github.com/osmosis-labs/sqs/sqsutil/sqshttp"
)

func TestDecodeJSON(t *testing.T) {
	type payload struct {
		Value int `json:"value"`
	}

	tests := []struct {
		name     string
		body     string
		limit    int64
		want     int
		wantErr  bool
		tooLarge bool
	}{
		{name: "one value", body: `{"value":7}`, limit: 64, want: 7},
		{name: "surrounding whitespace", body: " \n{\"value\":7}\n", limit: 64, want: 7},
		{name: "body exactly at the limit", body: `{"value":7}`, limit: int64(len(`{"value":7}`)), want: 7},
		{name: "one byte over the limit", body: `{"value":7} `, limit: int64(len(`{"value":7}`)), wantErr: true, tooLarge: true},
		// A prefix that is itself valid JSON must not be accepted from an oversized body.
		{name: "valid prefix of an oversized body", body: `{"value":7}` + strings.Repeat(" ", 64), limit: 16, wantErr: true, tooLarge: true},
		{name: "trailing junk", body: `{"value":7}x`, limit: 64, wantErr: true},
		{name: "second document", body: `{"value":7}{"value":8}`, limit: 64, wantErr: true},
		{name: "empty body", body: ``, limit: 64, wantErr: true},
		{name: "invalid limit", body: `{"value":7}`, limit: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got payload
			err := sqshttp.DecodeJSON(strings.NewReader(tt.body), tt.limit, &got)
			if !tt.wantErr {
				require.NoError(t, err)
				require.Equal(t, tt.want, got.Value)
				return
			}
			require.Error(t, err)
			require.Equal(t, tt.tooLarge, errors.Is(err, sqshttp.ErrBodyTooLarge), err)
		})
	}
}

func TestGet(t *testing.T) {
	serve := func(status int, body string) string {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(server.Close)
		return server.URL
	}

	t.Run("decodes pool fees", func(t *testing.T) {
		url := serve(http.StatusOK, `{"last_update_at":5,"data":[{"pool_id":"1","volume_24h":2.5,"fees_percentage":"0.2%"}]}`)
		got, err := sqshttp.Get[sqspassthroughdomain.PoolFees](http.DefaultClient, url, "/fees")
		require.NoError(t, err)
		require.Equal(t, int64(5), got.LastUpdateAt)
		require.Equal(t, []sqspassthroughdomain.PoolFee{{PoolID: "1", Volume24h: 2.5, FeesPercentage: "0.2%"}}, got.Data)
	})

	t.Run("non-2xx status is an error even with a JSON body", func(t *testing.T) {
		url := serve(http.StatusServiceUnavailable, `{"data":[]}`)
		_, err := sqshttp.Get[sqspassthroughdomain.PoolFees](http.DefaultClient, url, "/fees")
		require.ErrorContains(t, err, "503")
	})

	t.Run("oversized body", func(t *testing.T) {
		url := serve(http.StatusOK, `{"data":[],"pad":"`+strings.Repeat("x", sqshttp.MaxResponseBytes)+`"}`)
		_, err := sqshttp.Get[sqspassthroughdomain.PoolFees](http.DefaultClient, url, "/fees")
		require.ErrorIs(t, err, sqshttp.ErrBodyTooLarge)
	})
}
