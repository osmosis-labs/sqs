package mocks

import (
	"context"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/sqs/domain"
)

type QuoteSimulatorMock struct {
	SimulateQuoteFn func(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier math.LegacyDec, simulatorAddress string) (uint64, types.Coin, error)
}

// SimulateQuote implements domain.QuoteSimulator.
func (q *QuoteSimulatorMock) SimulateQuote(ctx context.Context, quote domain.Quote, slippageToleranceMultiplier math.LegacyDec, simulatorAddress string) (uint64, types.Coin, error) {
	if q.SimulateQuoteFn != nil {
		return q.SimulateQuoteFn(ctx, quote, slippageToleranceMultiplier, simulatorAddress)
	}
	panic("SimulateQuoteFn not implemented")
}

var _ domain.QuoteSimulator = &QuoteSimulatorMock{}
