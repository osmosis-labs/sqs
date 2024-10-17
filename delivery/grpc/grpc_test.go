package grpc_test

import (
	"testing"

	"github.com/osmosis-labs/sqs/delivery/grpc"

	"github.com/stretchr/testify/assert"
)

// TestNewClient tests the NewClient function
func TestNewClient(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{
			name:     "Valid endpoint",
			endpoint: "localhost:9090",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := grpc.NewClient(tt.endpoint)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, client)
				assert.NotNil(t, client.ClientConn)
			}
		})
	}
}
