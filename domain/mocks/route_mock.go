package mocks

import (
	"context"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
)

type RouteMock struct {
	CalculateTokenOutByTokenInFunc      func(ctx context.Context, tokenIn types.Coin) (types.Coin, error)
	ContainsGeneralizedCosmWasmPoolFunc func() bool
	GetPoolsFunc                        func() []domain.RoutablePool
	GetTokenOutDenomFunc                func() string
	GetTokenInDenomFunc                 func() string
	PrepareResultPoolsFunc              func(ctx context.Context, tokenIn types.Coin, logger log.Logger) ([]domain.RoutablePool, math.LegacyDec, math.LegacyDec, error)
	StringFunc                          func() string

	GetAmountInFunc  func() math.Int
	GetAmountOutFunc func() math.Int
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

// GetTokenInDenom implements domain.Route.
func (r *RouteMock) GetTokenInDenom() string {
	if r.GetTokenInDenomFunc != nil {
		return r.GetTokenInDenomFunc()
	}

	panic("unimplemented")
}

// PrepareResultPools implements domain.Route.
func (r *RouteMock) PrepareResultPools(ctx context.Context, tokenIn types.Coin, logger log.Logger) ([]domain.RoutablePool, math.LegacyDec, math.LegacyDec, error) {
	if r.PrepareResultPoolsFunc != nil {
		return r.PrepareResultPoolsFunc(ctx, tokenIn, logger)
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

func (r *RouteMock) GetAmountIn() math.Int {
	if r.GetAmountInFunc != nil {
		return r.GetAmountInFunc()
	}

	panic("unimplemented")
}

func (r *RouteMock) GetAmountOut() math.Int {
	if r.GetAmountOutFunc != nil {
		return r.GetAmountOutFunc()
	}

	panic("unimplemented")
}

var _ domain.Route = &RouteMock{}
var _ domain.SplitRoute = &RouteMock{}
