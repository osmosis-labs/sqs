// Package types provides types and client implementations for interacting with the Cosmos Auth module.
package types

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/gogoproto/grpc"
	"github.com/osmosis-labs/osmosis/v28/app"
)

var (
	encodingConfig = app.MakeEncodingConfig()
)

// QueryClient is the client API for Query service.
type QueryClient interface {
	// GetAccount retrieves account information for a given address.
	GetAccount(ctx context.Context, address string) (*authtypes.BaseAccount, error)
}

// NewQueryClient creates a new QueryClient instance with the provided LCD (Light Client Daemon) endpoint.
func NewQueryClient(conn grpc.ClientConn) QueryClient {
	return &queryClient{
		queryClient: authtypes.NewQueryClient(conn),
	}
}

var _ QueryClient = &queryClient{}

// queryClient is an implementation of the QueryClient interface.
type queryClient struct {
	queryClient authtypes.QueryClient
}

// GetAccount returns the account information for the given address.
func (c *queryClient) GetAccount(ctx context.Context, address string) (*authtypes.BaseAccount, error) {
	resp, err := c.queryClient.Account(ctx, &authtypes.QueryAccountRequest{
		Address: address,
	})
	if err != nil {
		return nil, err
	}

	var account types.AccountI
	if err := encodingConfig.InterfaceRegistry.UnpackAny(resp.Account, &account); err != nil {
		return nil, err
	}

	baseAccount, ok := account.(*authtypes.BaseAccount)
	if !ok {
		return nil, fmt.Errorf("account is not of the type of BaseAccount")
	}

	return baseAccount, nil
}
