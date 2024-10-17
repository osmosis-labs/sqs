package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain/cosmos/auth/types"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

var _ types.QueryClient = &AuthQueryClientMock{}

type AuthQueryClientMock struct {
	GetAccountFunc func(ctx context.Context, address string) (*authtypes.BaseAccount, error)
}

func (m *AuthQueryClientMock) GetAccount(ctx context.Context, address string) (*authtypes.BaseAccount, error) {
	if m.GetAccountFunc != nil {
		return m.GetAccountFunc(ctx, address)
	}
	panic("GetAccountFunc has not been mocked")
}

func (m *AuthQueryClientMock) WithGetAccount(account *authtypes.BaseAccount, err error) {
	m.GetAccountFunc = func(ctx context.Context, address string) (*authtypes.BaseAccount, error) {
		return account, err
	}
}
