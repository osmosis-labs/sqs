package chainpricing

import (
	"context"
	"strings"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type chainPricing struct {
	TUsecase mvc.TokensUsecase
	RUsecase mvc.RouterUsecase

	cache       *cache.Cache
	cacheExpiry time.Duration
}

var _ domain.PricingStrategy = &chainPricing{}

func New(routerUseCase mvc.RouterUsecase, tokenUseCase mvc.TokensUsecase) domain.PricingStrategy {
	return &chainPricing{
		RUsecase: routerUseCase,
		TUsecase: tokenUseCase,

		cache: cache.New(),
		// TODO: move to config.
		cacheExpiry: 2 * time.Second,
	}
}

// GetPrice implements pricing.PricingStrategy.
func (c *chainPricing) GetPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// equal base and quote yield the price of one
	if baseDenom == quoteDenom {
		return osmomath.OneBigDec(), nil
	}

	cacheKey := formatCacheKey(baseDenom, quoteDenom)

	cachedValue, found := c.cache.Get(cacheKey)
	cachedBigDecPrice, ok := cachedValue.(osmomath.BigDec)

	if found && ok {
		// TODO: add cache hit telemetry
		return cachedBigDecPrice, nil
	} else if !found {
		// TODO telemetry
	} else {
		// TODO: temetry
	}

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

	c.cache.Set(cacheKey, currentPrice, c.cacheExpiry)

	return currentPrice, nil
}

func formatCacheKey(a, b string) string {
	if a < b {
		a, b = b, a
	}

	var sb strings.Builder
	sb.WriteString(a)
	sb.WriteString(b)
	return sb.String()
}
