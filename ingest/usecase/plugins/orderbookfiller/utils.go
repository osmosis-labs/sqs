package orderbookfiller

import (
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"
	blockctx "github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbookfiller/context/block"
	"go.uber.org/zap"
)

// usdcToDenomValueScaled converts the desired USDC value to the equivalent value in the base denom.
// Applies the scaling factor.
// Returns error if:
// - Price for base denom is not found
// - Scaling factor for base denom is not found.
func (o *orderbookFillerIngestPlugin) usdcToDenomValueScaled(denomToValue string, desiredUSDCValue osmomath.Dec, prices domain.PricesResult) (osmomath.Int, error) {
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
func (o *orderbookFillerIngestPlugin) shouldSkipLowBalance(baseDenom string, baseAmountBalance osmomath.Int, prices domain.PricesResult) (bool, error) {
	minBaseValue, err := o.usdcToDenomValueScaled(baseDenom, orderbookplugindomain.MinBalanceValueInUSDC, prices)
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

// validateUserBalances validates the user balances meeting the minimum configured theshold in quote denom terms.
// Returns error if fails to validate or if balance is low.
// Returns nil on success.
func (o *orderbookFillerIngestPlugin) validateUserBalances(ctx blockctx.BlockCtxI, baseDenom, quoteDenom string) error {
	userBlockBalances := ctx.GetUserBalances()
	blockPrices := ctx.GetPrices()

	// Validate base denom balance
	baseAmountBalance := userBlockBalances.AmountOf(baseDenom)
	isBaseLowBalance, err := o.shouldSkipLowBalance(baseDenom, baseAmountBalance, blockPrices)
	if err != nil {
		return err
	}

	if isBaseLowBalance {
		return fmt.Errorf("base denom (%s) balance is below the configured threshold %s", baseDenom, baseAmountBalance)
	}

	// Validate quote denom balance.
	quoteAmountBalance := userBlockBalances.AmountOf(quoteDenom)
	isQuoteLowBalance, err := o.shouldSkipLowBalance(quoteDenom, quoteAmountBalance, blockPrices)
	if err != nil {
		return err
	}

	if isQuoteLowBalance {
		return fmt.Errorf("quote denom (%s) balance is below the configured threshold %s", quoteDenom, quoteAmountBalance)
	}

	return nil
}
