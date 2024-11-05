package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/sqsdomain"

	"github.com/osmosis-labs/osmosis/osmomath"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ mvc.RouterUsecase = &RouterUsecaseMock{}

// RouterUsecaseMock is a mock implementation of the RouterUsecase interface
type RouterUsecaseMock struct {
	GetSimpleQuoteFunc                           func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error)
	GetPoolSpotPriceFunc                         func(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error)
	GetOptimalQuoteFunc                          func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error)
	GetOptimalQuoteInGivenOutFunc                func(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, opts ...domain.RouterOption) (domain.Quote, error)
	GetBestSingleRouteQuoteFunc                  func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, error)
	GetCustomDirectQuoteFunc                     func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error)
	GetCustomDirectQuoteMultiPoolFunc            func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error)
	GetCustomDirectQuoteMultiPoolInGivenOutFunc  func(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error)
	GetCandidateRoutesFunc                       func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sqsdomain.CandidateRoutes, error)
	GetTakerFeeFunc                              func(poolID uint64) ([]sqsdomain.TakerFeeForPair, error)
	SetTakerFeesFunc                             func(takerFees sqsdomain.TakerFeeMap)
	GetCachedCandidateRoutesFunc                 func(ctx context.Context, tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, bool, error)
	StoreRouterStateFilesFunc                    func() error
	GetRouterStateFunc                           func() (domain.RouterState, error)
	GetSortedPoolsFunc                           func() []sqsdomain.PoolI
	GetConfigFunc                                func() domain.RouterConfig
	ConvertMinTokensPoolLiquidityCapToFilterFunc func(minTokensPoolLiquidityCap uint64) uint64
	SetSortedPoolsFunc                           func(pools []sqsdomain.PoolI)
	GetMinPoolLiquidityCapFilterFunc             func(tokenInDenom string, tokenOutDenom string) (uint64, error)

	BaseFee domain.BaseFee
}

// GetBaseFee implements mvc.RouterUsecase.
func (m *RouterUsecaseMock) GetBaseFee() domain.BaseFee {
	return m.BaseFee
}

// GetMinPoolLiquidityCapFilter implements mvc.RouterUsecase.
func (m *RouterUsecaseMock) GetMinPoolLiquidityCapFilter(tokenInDenom string, tokenOutDenom string) (uint64, error) {
	if m.GetMinPoolLiquidityCapFilterFunc != nil {
		return m.GetMinPoolLiquidityCapFilterFunc(tokenInDenom, tokenOutDenom)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetSimpleQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
	if m.GetSimpleQuoteFunc != nil {
		return m.GetSimpleQuoteFunc(ctx, tokenIn, tokenOutDenom, opts...)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetPoolSpotPrice(ctx context.Context, poolID uint64, quoteAsset, baseAsset string) (osmomath.BigDec, error) {
	if m.GetPoolSpotPriceFunc != nil {
		return m.GetPoolSpotPriceFunc(ctx, poolID, quoteAsset, baseAsset)
	}
	return osmomath.BigDec{}, nil
}

func (m *RouterUsecaseMock) GetOptimalQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
	if m.GetOptimalQuoteFunc != nil {
		return m.GetOptimalQuoteFunc(ctx, tokenIn, tokenOutDenom, opts...)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetOptimalQuoteInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
	if m.GetOptimalQuoteInGivenOutFunc != nil {
		return m.GetOptimalQuoteInGivenOutFunc(ctx, tokenOut, tokenInDenom, opts...)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetBestSingleRouteQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (domain.Quote, error) {
	if m.GetBestSingleRouteQuoteFunc != nil {
		return m.GetBestSingleRouteQuoteFunc(ctx, tokenIn, tokenOutDenom)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetCustomDirectQuote(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, poolID uint64) (domain.Quote, error) {
	if m.GetCustomDirectQuoteFunc != nil {
		return m.GetCustomDirectQuoteFunc(ctx, tokenIn, tokenOutDenom, poolID)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetCustomDirectQuoteMultiPool(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom []string, poolIDs []uint64) (domain.Quote, error) {
	if m.GetCustomDirectQuoteMultiPoolFunc != nil {
		return m.GetCustomDirectQuoteMultiPoolFunc(ctx, tokenIn, tokenOutDenom, poolIDs)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetCustomDirectQuoteMultiPoolInGivenOut(ctx context.Context, tokenOut sdk.Coin, tokenInDenom []string, poolIDs []uint64) (domain.Quote, error) {
	if m.GetCustomDirectQuoteMultiPoolInGivenOutFunc != nil {
		return m.GetCustomDirectQuoteMultiPoolInGivenOutFunc(ctx, tokenOut, tokenInDenom, poolIDs)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) GetCandidateRoutes(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string) (sqsdomain.CandidateRoutes, error) {
	if m.GetCandidateRoutesFunc != nil {
		return m.GetCandidateRoutesFunc(ctx, tokenIn, tokenOutDenom)
	}
	return sqsdomain.CandidateRoutes{}, nil
}

func (m *RouterUsecaseMock) GetTakerFee(poolID uint64) ([]sqsdomain.TakerFeeForPair, error) {
	if m.GetTakerFeeFunc != nil {
		return m.GetTakerFeeFunc(poolID)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) SetTakerFees(takerFees sqsdomain.TakerFeeMap) {
	if m.SetTakerFeesFunc != nil {
		m.SetTakerFeesFunc(takerFees)
	}
}

func (m *RouterUsecaseMock) GetCachedCandidateRoutes(ctx context.Context, tokenInDenom, tokenOutDenom string) (sqsdomain.CandidateRoutes, bool, error) {
	if m.GetCachedCandidateRoutesFunc != nil {
		return m.GetCachedCandidateRoutesFunc(ctx, tokenInDenom, tokenOutDenom)
	}
	return sqsdomain.CandidateRoutes{}, false, nil
}

func (m *RouterUsecaseMock) StoreRouterStateFiles() error {
	if m.StoreRouterStateFilesFunc != nil {
		return m.StoreRouterStateFilesFunc()
	}
	return nil
}

func (m *RouterUsecaseMock) GetRouterState() (domain.RouterState, error) {
	if m.GetRouterStateFunc != nil {
		return m.GetRouterStateFunc()
	}
	return domain.RouterState{}, nil
}

func (m *RouterUsecaseMock) GetSortedPools() []sqsdomain.PoolI {
	if m.GetSortedPoolsFunc != nil {
		return m.GetSortedPoolsFunc()
	}
	return nil
}

func (m *RouterUsecaseMock) GetConfig() domain.RouterConfig {
	if m.GetConfigFunc != nil {
		return m.GetConfigFunc()
	}
	return domain.RouterConfig{}
}

func (m *RouterUsecaseMock) ConvertMinTokensPoolLiquidityCapToFilter(minTokensPoolLiquidityCap uint64) uint64 {
	if m.ConvertMinTokensPoolLiquidityCapToFilterFunc != nil {
		return m.ConvertMinTokensPoolLiquidityCapToFilterFunc(minTokensPoolLiquidityCap)
	}
	panic("unimplemented")
}

func (m *RouterUsecaseMock) SetSortedPools(pools []sqsdomain.PoolI) {
	if m.SetSortedPoolsFunc != nil {
		m.SetSortedPoolsFunc(pools)
	}
}
