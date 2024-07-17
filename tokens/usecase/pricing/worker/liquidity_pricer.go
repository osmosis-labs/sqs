package worker

import (
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
)

type liquidityPricer struct {
	defaultQuoteDenom string

	scalingFactorGetterCb domain.ScalingFactorGetterCb
}

const liquidityCapErrorSeparator = "; "

var _ domain.LiquidityPricer = &liquidityPricer{}

func NewLiquidityPricer(defaultQuoteDenom string, chainScalingFactorGetterCb domain.ScalingFactorGetterCb) domain.LiquidityPricer {
	return &liquidityPricer{
		defaultQuoteDenom: defaultQuoteDenom,

		scalingFactorGetterCb: chainScalingFactorGetterCb,
	}
}

// PriceCoin implements domain.PoolLiquidityPricerWorker.
func (p *liquidityPricer) PriceCoin(coin sdk.Coin, price osmomath.BigDec) osmomath.Dec {
	if price.IsZero() {
		// If the price is zero, set the capitalization to zero.
		return osmomath.ZeroDec()
	}

	// Get the scaling factor for the base denom.
	baseScalingFactor, err := p.scalingFactorGetterCb(coin.Denom)
	if err != nil {
		// If there is an error, keep the total liquidity but set the capitalization to zero.
		return osmomath.ZeroDec()
	}

	priceInfo := domain.DenomPriceInfo{
		Price:         price,
		ScalingFactor: baseScalingFactor,
	}

	liquidityCapitalization, err := ComputeCoinCap(coin, priceInfo)
	if err != nil {
		// If there is an error, keep the total liquidity but set the capitalization to zero.
		return osmomath.ZeroDec()
	}

	return liquidityCapitalization
}

// PriceBalances implements domain.PoolLiquidityPricerWorker.
func (p *liquidityPricer) PriceBalances(balances sdk.Coins, prices domain.PricesResult) (osmomath.Int, string) {
	totalCapitalization := osmomath.ZeroInt()

	// Note: errors may occur in any denom.
	// As a result, we accumulate them in this error string
	// to ease debugging if issues occur.
	liquidityCapErrorStr := ""

	for _, balance := range balances {
		denom := balance.Denom

		price := prices.GetPriceForDenom(denom, p.defaultQuoteDenom)

		currentCapitalization := p.PriceCoin(balance, price)

		if currentCapitalization.IsZero() {
			if len(liquidityCapErrorStr) != 0 {
				liquidityCapErrorStr += liquidityCapErrorSeparator
			}

			liquidityCapErrorStr += formatLiquidityCapErrorStr(denom)
		}

		totalCapitalization = totalCapitalization.Add(currentCapitalization.TruncateInt())
	}

	return totalCapitalization, liquidityCapErrorStr
}

// formatLiquidityCapErrorStr formats the liquidity cap error
func formatLiquidityCapErrorStr(denom string) string {
	return fmt.Sprintf("zero cap for denom (%s)", denom)
}

// ComputeCoinCap computes the equivalent of the given coin in the desired quote denom that is set on ingester.
//
// Returns error if:
// * Price is zero
// * Scaling factor is zero
// * Truncation occurs in intermediary operations. Truncation is defined as the original amount
// being non-zero and the computed amount being zero.
func ComputeCoinCap(coin sdk.Coin, baseDenomPriceData domain.DenomPriceInfo) (math.LegacyDec, error) {
	if baseDenomPriceData.Price.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("price for %s is zero", coin.Denom)
	}
	if baseDenomPriceData.ScalingFactor.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("scaling factor for %s is zero", coin.Denom)
	}

	currentCoinCap := osmomath.BigDecFromSDKInt(coin.Amount).MulMut(baseDenomPriceData.Price).QuoMut(osmomath.BigDecFromDec(baseDenomPriceData.ScalingFactor))
	isOriginalAmountZero := coin.Amount.IsZero()

	// Truncation in intermediary operation - return error.
	if currentCoinCap.IsZero() && !isOriginalAmountZero {
		return osmomath.Dec{}, fmt.Errorf("truncation occurred when multiplying (%s) of denom (%s) by the scaling factor (%s)", currentCoinCap, coin.Denom, baseDenomPriceData.ScalingFactor)
	}

	return currentCoinCap.Dec(), nil
}
