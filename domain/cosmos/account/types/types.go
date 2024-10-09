package types

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/osmosis-labs/sqs/delivery/http"
)

// QueryClient is the client API for Query service.
type QueryClient interface {
	GetAccount(ctx context.Context, address string) (*QueryAccountResponse, error)
}

func NewQueryClient(lcd string) QueryClient {
	return &queryClient{lcd}
}


var _ QueryClient = &queryClient{}

type queryClient struct {
	lcd string
}

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

type Account struct {
	Sequence      uint64 `json:"sequence"`
	AccountNumber uint64 `json:"account_number"`
}
type QueryAccountResponse struct {
	Account Account `json:"account"`
}
