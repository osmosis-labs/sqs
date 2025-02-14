package mocks

import (
	"context"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/sqs/domain"
)

type QuoteSimulatorMock struct {
	SimulateQuoteFn func(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier math.LegacyDec, simulatorAddress string) domain.TxFeeInfo
}

// SimulateQuoteOutGivenIn implements domain.QuoteSimulator.
func (q *QuoteSimulatorMock) SimulateQuoteOutGivenIn(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier math.LegacyDec, simulatorAddress string) domain.TxFeeInfo {
	if q.SimulateQuoteFn != nil {
		return q.SimulateQuoteFn(ctx, quote, slippageToleranceMultiplier, simulatorAddress)
	}
	panic("SimulateQuoteFn not implemented")
}

var _ domain.QuoteSimulator = &QuoteSimulatorMock{}
