package domain_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"

	"github.com/osmosis-labs/sqs/domain"
)

type usdPriceStub struct {
	price osmomath.BigDec
	err   error
}

func (s usdPriceStub) GetPrice(context.Context, string, string, ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	return s.price, nil, s.err
}

func TestQuoteUSDPegChecker_Check(t *testing.T) {
	cfg := domain.QuoteUSDPegConfig{Enabled: true, WarnDeviation: 0.01, SevereDeviation: 0.05}

	tests := map[string]struct {
		source        usdPriceStub
		expectedState string
		hasPrice      bool
	}{
		"severe depeg":                {usdPriceStub{price: osmomath.MustNewBigDecFromStr("0.94")}, domain.QuoteUSDPegStatusSevere, true},
		"warn depeg above the dollar": {usdPriceStub{price: osmomath.MustNewBigDecFromStr("1.02")}, domain.QuoteUSDPegStatusWarn, true},
		"within warn threshold":       {usdPriceStub{price: osmomath.MustNewBigDecFromStr("0.995")}, domain.QuoteUSDPegStatusOK, true},
		"reference unavailable":       {usdPriceStub{err: errors.New("stale")}, domain.QuoteUSDPegStatusUnknown, false},
		"zero reference is not a peg": {usdPriceStub{price: osmomath.ZeroBigDec()}, domain.QuoteUSDPegStatusUnknown, false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			checker, err := domain.NewQuoteUSDPegChecker(cfg, "allusdc", tc.source)
			require.NoError(t, err)

			peg := checker.Check(context.Background())
			require.Equal(t, tc.expectedState, peg.Status)
			require.Equal(t, tc.hasPrice, peg.Price != nil && peg.Deviation != nil)
		})
	}

	require.Nil(t, (*domain.QuoteUSDPegChecker)(nil).Check(context.Background()))
}
