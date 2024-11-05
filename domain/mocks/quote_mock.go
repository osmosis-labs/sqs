package mocks

import (
	"context"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
)

type MockQuote struct {
	GetAmountInFunc  func() types.Coin
	GetAmountOutFunc func() math.Int
	GetRouteFunc     func() []domain.SplitRoute
}

// GetAmountIn implements domain.Quote.
func (m *MockQuote) GetAmountIn() types.Coin {
	if m.GetAmountInFunc != nil {
		return m.GetAmountInFunc()
	}

	panic("unimplemented")
}

// GetAmountOut implements domain.Quote.
func (m *MockQuote) GetAmountOut() math.Int {
	if m.GetAmountOutFunc != nil {
		return m.GetAmountOutFunc()
	}

	panic("unimplemented")
}

// GetEffectiveFee implements domain.Quote.
func (m *MockQuote) GetEffectiveFee() math.LegacyDec {
	panic("unimplemented")
}

// GetInBaseOutQuoteSpotPrice implements domain.Quote.
func (m *MockQuote) GetInBaseOutQuoteSpotPrice() math.LegacyDec {
	panic("unimplemented")
}

// GetPriceImpact implements domain.Quote.
func (m *MockQuote) GetPriceImpact() math.LegacyDec {
	panic("unimplemented")
}

// GetRoute implements domain.Quote.
func (m *MockQuote) GetRoute() []domain.SplitRoute {
	if m.GetRouteFunc != nil {
		return m.GetRouteFunc()
	}

	panic("unimplemented")
}

// PrepareResult implements domain.Quote.
func (m *MockQuote) PrepareResult(ctx context.Context, scalingFactor math.LegacyDec, logger log.Logger) ([]domain.SplitRoute, math.LegacyDec, error) {
	panic("unimplemented")
}

// SetQuotePriceInfo implements domain.Quote.
func (m *MockQuote) SetQuotePriceInfo(info *domain.TxFeeInfo) {
	panic("unimplemented")
}

// String implements domain.Quote.
func (m *MockQuote) String() string {
	panic("unimplemented")
}

var _ domain.Quote = &MockQuote{}
