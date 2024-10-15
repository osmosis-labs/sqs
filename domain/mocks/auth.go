package mocks

import (
	"context"

	authtypes "github.com/osmosis-labs/sqs/domain/cosmos/auth/types"
)

var _ authtypes.QueryClient = &AuthQueryClientMock{}

type AuthQueryClientMock struct {
	GetAccountFunc func(ctx context.Context, address string) (*authtypes.QueryAccountResponse, error)
}

func (m *AuthQueryClientMock) GetAccount(ctx context.Context, address string) (*authtypes.QueryAccountResponse, error) {
	if m.GetAccountFunc != nil {
		return m.GetAccountFunc(ctx, address)
	}
	panic("GetAccountFunc has not been mocked")
}

func (m *AuthQueryClientMock) WithGetAccount(response *authtypes.QueryAccountResponse, err error) {
	m.GetAccountFunc = func(ctx context.Context, address string) (*authtypes.QueryAccountResponse, error) {
		return response, err
	}
}
