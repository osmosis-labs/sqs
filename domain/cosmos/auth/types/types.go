// Package types provides types and client implementations for interacting with the Cosmos Auth module.
package types

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/osmosis-labs/sqs/delivery/http"
)

// QueryClient is the client API for Query service.
type QueryClient interface {
	// GetAccount retrieves account information for a given address.
	GetAccount(ctx context.Context, address string) (*QueryAccountResponse, error)
}

// NewQueryClient creates a new QueryClient instance with the provided LCD (Light Client Daemon) endpoint.
func NewQueryClient(lcd string) QueryClient {
	return &queryClient{lcd}
}

var _ QueryClient = &queryClient{}

// queryClient is an implementation of the QueryClient interface.
type queryClient struct {
	lcd string
}

// Account represents the basic account information.
type Account struct {
	Sequence      uint64 `json:"sequence"`       // Current sequence (nonce) of the account, used to prevent replay attacks.
	AccountNumber uint64 `json:"account_number"` // Unique identifier of the account on the blockchain.
}

// QueryAccountResponse encapsulates the response for an account query.
type QueryAccountResponse struct {
	Account Account `json:"account"`
}

// GetAccount retrieves account information for a given address.
func (c *queryClient) GetAccount(ctx context.Context, address string) (*QueryAccountResponse, error) {
	resp, err := http.Get(ctx, c.lcd+"/cosmos/auth/v1beta1/accounts/"+address)
	if err != nil {
		return nil, err
	}

	type queryAccountResponse struct {
		Account struct {
			Sequence      string `json:"sequence"`
			AccountNumber string `json:"account_number"`
		} `json:"account"`
	}

	var accountRes queryAccountResponse
	err = json.Unmarshal(resp, &accountRes)
	if err != nil {
		return nil, err
	}

	sequence, err := strconv.ParseUint(accountRes.Account.Sequence, 10, 64)
	if err != nil {
		return nil, err
	}

	accountNumber, err := strconv.ParseUint(accountRes.Account.AccountNumber, 10, 64)
	if err != nil {
		return nil, err
	}

	return &QueryAccountResponse{
		Account: Account{
			Sequence:      sequence,
			AccountNumber: accountNumber,
		},
	}, nil
}
