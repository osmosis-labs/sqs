package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sqshttp "github.com/osmosis-labs/sqs/delivery/http"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	tests := []struct {
		name           string
		url            string
		expectedBody   string
		timeout        time.Duration
		serverResponse func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:         "Success",
			url:          "/success",
			expectedBody: "Hello, World!",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte("Hello, World!"))
			},
		},
		{
			name:         "Timeout",
			url:          "/timeout",
			expectedBody: "",
			timeout:      10 * time.Millisecond,
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(20 * time.Millisecond)
				w.Write([]byte("Too late"))
			},
		},
		{
			name:         "Server Error",
			url:          "/error",
			expectedBody: "Internal Server Error\n",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			},
		},
	}

	defaultTimeout := sqshttp.DefaultClient.Timeout
	resetClient := func() {
		sqshttp.DefaultClient.Timeout = defaultTimeout
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			sqshttp.DefaultClient.Timeout = tt.timeout
			defer resetClient()

			ctx := context.Background()
			body, err := sqshttp.Get(ctx, server.URL+tt.url)
			assert.NoError(t, err)
			assert.Equal(t, string(body), tt.expectedBody)

		})
	}
}
