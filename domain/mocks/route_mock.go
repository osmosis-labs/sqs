package mocks

import (
	"context"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
)

type RouteMock struct {
	CalculateTokenOutByTokenInFunc      func(ctx context.Context, tokenIn types.Coin) (types.Coin, error)
	ContainsGeneralizedCosmWasmPoolFunc func() bool
	GetPoolsFunc                        func() []domain.RoutablePool
	GetTokenOutDenomFunc                func() string
	PrepareResultPoolsFunc              func(ctx context.Context, tokenIn types.Coin) ([]domain.RoutablePool, math.LegacyDec, math.LegacyDec, error)
	StringFunc                          func() string
}

// CalculateTokenOutByTokenIn implements domain.Route.
func (r *RouteMock) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn types.Coin) (types.Coin, error) {
	if r.CalculateTokenOutByTokenInFunc != nil {
		return r.CalculateTokenOutByTokenInFunc(ctx, tokenIn)
	}

	panic("unimplemented")
}

// ContainsGeneralizedCosmWasmPool implements domain.Route.
func (r *RouteMock) ContainsGeneralizedCosmWasmPool() bool {
	if r.ContainsGeneralizedCosmWasmPoolFunc != nil {
		return r.ContainsGeneralizedCosmWasmPoolFunc()
	}

	panic("unimplemented")
}

// GetPools implements domain.Route.
func (r *RouteMock) GetPools() []domain.RoutablePool {
	if r.GetPoolsFunc != nil {
		return r.GetPoolsFunc()
	}

	panic("unimplemented")
}

// GetTokenOutDenom implements domain.Route.
func (r *RouteMock) GetTokenOutDenom() string {
	if r.GetTokenOutDenomFunc != nil {
		return r.GetTokenOutDenomFunc()
	}

	panic("unimplemented")
}

// PrepareResultPools implements domain.Route.
func (r *RouteMock) PrepareResultPools(ctx context.Context, tokenIn types.Coin) ([]domain.RoutablePool, math.LegacyDec, math.LegacyDec, error) {
	if r.PrepareResultPoolsFunc != nil {
		return r.PrepareResultPoolsFunc(ctx, tokenIn)
	}

	panic("unimplemented")
}

// String implements domain.Route.
func (r *RouteMock) String() string {
	if r.StringFunc != nil {
		return r.StringFunc()
	}

	panic("unimplemented")
}

var _ domain.Route = &RouteMock{}
