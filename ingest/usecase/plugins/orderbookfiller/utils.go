package orderbookfiller

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"go.uber.org/zap"
)

// usdcToDenomVlaue converts the desired USDC value to the equivalent value in the base denom.
func (o *orderbookFillerIngestPlugin) usdcToDenomVlaue(denomToValue string, desiredUSDCValue osmomath.Dec, prices domain.PricesResult) (osmomath.Int, error) {
	price := prices.GetPriceForDenom(denomToValue, o.defaultQuoteDenom)
	if price.IsZero() {
		return osmomath.Int{}, fmt.Errorf("price not found for %s", denomToValue)
	}

	// Base scaling factor
	scalingFactor, err := o.tokensUseCase.GetChainScalingFactorByDenomMut(denomToValue)
	if err != nil {
		return osmomath.Int{}, err
	}

	// Scale the base amount
	denomValue := osmomath.BigDecFromDecMut(desiredUSDCValue.Mul(scalingFactor)).Quo(price)

	return denomValue.Dec().TruncateInt(), nil
}

// shouldSkipLowBalance checks if the base balance is below the minimum balance value in USDC.
func (o *orderbookFillerIngestPlugin) shouldSkipLowBalance(baseDenom string, baseAmountBalance, minBalanceValueInUSDC osmomath.Int, prices domain.PricesResult) (bool, error) {
	minBaseValue, err := o.usdcToDenomVlaue(baseDenom, minBalanceValueInUSDC.ToLegacyDec(), prices)
	if err != nil {
		o.logger.Error("failed to convert USDC to base value", zap.Error(err))
		return false, err
	}

	if baseAmountBalance.LT(minBaseValue) {
		o.logger.Info("skipping orderbook processing due to low balance", zap.String("denom", baseDenom), zap.Stringer("balance", baseAmountBalance), zap.Stringer("min_balance", minBaseValue))
		return true, nil
	}

	return false, nil
}

func (o *orderbookFillerIngestPlugin) getGasFee(blockCtx blockContext) (baseFee osmomath.BigDec) {
	// This is the min arb gas fee
	return osmomath.MustNewBigDecFromStr("0.1")
}
