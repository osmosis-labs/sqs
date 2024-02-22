package chainpricing

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type chainPricing struct {
	TUsecase mvc.TokensUsecase
	RUsecase mvc.RouterUsecase
}

var _ domain.PricingStrategy = &chainPricing{}

func New(routerUseCase mvc.RouterUsecase, tokenUseCase mvc.TokensUsecase) domain.PricingStrategy {
	return &chainPricing{
		RUsecase: routerUseCase,
		TUsecase: tokenUseCase,
	}
}

// GetPrice implements pricing.PricingStrategy.
func (c *chainPricing) GetPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// Get on-chain scaling factor for base denom.
	baseDenomScalingFactor, err := c.TUsecase.GetChainScalingFactorByDenomMut(ctx, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Get on-chain scaling factor for quote denom.
	quoteDenomScalingFactor, err := c.TUsecase.GetChainScalingFactorByDenomMut(ctx, quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Create a quote denom coin.
	oneQuoteCoin := sdk.NewCoin(quoteDenom, quoteDenomScalingFactor.TruncateInt())

	// Compute a quote for one quote coin.
	quote, err := c.RUsecase.GetOptimalQuote(ctx, oneQuoteCoin, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	// Compute on-chain price for 1 unit of base denom and quote denom.
	chainPrice := osmomath.NewBigDecFromBigInt(oneQuoteCoin.Amount.BigIntMut()).QuoMut(osmomath.NewBigDecFromBigInt(quote.GetAmountOut().BigIntMut()))

	// Compute precision scaling factor.
	precisionScalingFactor := osmomath.BigDecFromDec(baseDenomScalingFactor.Quo(oneQuoteCoin.Amount.ToLegacyDec()))

	// Apply scaling facors to descale the amounts to real amounts.
	currentPrice := chainPrice.MulMut(precisionScalingFactor)

	return currentPrice, nil
}
