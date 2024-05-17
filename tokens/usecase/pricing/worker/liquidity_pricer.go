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

	quoteDenomScalingFactor osmomath.BigDec
}

var _ domain.LiquidityPricer = &liquidityPricer{}

func NewLiquidityPricer(defaultQuoteDenom string, quoteDenomScalingFactor osmomath.Dec) domain.LiquidityPricer {
	return &liquidityPricer{
		defaultQuoteDenom:       defaultQuoteDenom,
		quoteDenomScalingFactor: osmomath.BigDecFromDec(quoteDenomScalingFactor),
	}
}

// ComputeCoinCap implements LiquidityPricer.
func (l *liquidityPricer) ComputeCoinCap(coin sdk.Coin, baseDenomPriceData domain.DenomPriceInfo) (math.LegacyDec, error) {
	if baseDenomPriceData.Price.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("price for %s is zero", coin.Denom)
	}
	if baseDenomPriceData.ScalingFactor.IsZero() {
		return osmomath.Dec{}, fmt.Errorf("scaling factor for %s is zero", coin.Denom)
	}

	currentCoinCap := osmomath.NewBigDecFromBigInt(coin.Amount.BigIntMut()).MulMut(baseDenomPriceData.Price)
	isOriginalAmountZero := coin.Amount.IsZero()

	// Truncation in intermediary operation - return error.
	currentCoinCap = l.quoteDenomScalingFactor.Mul(currentCoinCap).QuoMut(osmomath.BigDecFromDec(baseDenomPriceData.ScalingFactor))
	if currentCoinCap.IsZero() && !isOriginalAmountZero {
		return osmomath.Dec{}, fmt.Errorf("truncation occurred when multiplying (%s) of denom (%s) by the scaling factor (%s)", currentCoinCap, coin.Denom, baseDenomPriceData.ScalingFactor)
	}

	return currentCoinCap.Dec(), nil
}
