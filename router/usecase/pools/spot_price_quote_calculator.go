package pools

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type spotPriceQuoteCalculator struct {
	scalingFactorGetterCb domain.ScalingFactorGetterCb
	quoteEstimatorCb      domain.QuoteEstimatorCb
}

var _ domain.SpotPriceQuoteCalculator = &spotPriceQuoteCalculator{}

// NewSpotPriceQuoteComputer returns a new spot price quote computer.s
func NewSpotPriceQuoteComputer(scalingFactorGetterCb domain.ScalingFactorGetterCb, quoteEstimatorCb domain.QuoteEstimatorCb) domain.SpotPriceQuoteCalculator {
	return &spotPriceQuoteCalculator{
		scalingFactorGetterCb: scalingFactorGetterCb,
		quoteEstimatorCb:      quoteEstimatorCb,
	}
}

// Calculate implements domain.SpotPriceQuoteCalculator
func (s *spotPriceQuoteCalculator) Calculate(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	quoteScalingFactor, err := s.scalingFactorGetterCb(quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Calculate the token out amount for the quote denom
	quoteCoin := sdk.NewCoin(quoteDenom, quoteScalingFactor.TruncateInt())
	out, err := s.quoteEstimatorCb(ctx, quoteCoin, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// If failed to compute out amount due to illiquidity (or any other reason), fail.
	if out.IsNil() || out.Amount.IsNil() || out.Amount.IsZero() {
		return osmomath.BigDec{}, domain.SpotPriceQuoteCalculatorOutAmountZeroError{
			QuoteCoinStr: quoteCoin.String(),
			BaseDenom:    baseDenom,
		}
	}

	// If no error, compute the spot price from quote
	spotPrice := osmomath.BigDecFromSDKInt(quoteCoin.Amount).QuoMut(osmomath.BigDecFromSDKInt(out.Amount))

	// If spot price truncated, return error
	if spotPrice.IsZero() {
		return osmomath.BigDec{}, domain.SpotPriceQuoteCalculatorTruncatedError{
			QuoteCoinStr: quoteCoin.String(),
			BaseDenom:    baseDenom,
		}
	}

	// If spot price is not zero, return it
	return spotPrice, nil
}
