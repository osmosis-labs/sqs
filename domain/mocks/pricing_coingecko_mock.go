package mocks

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	coingeckopricing "github.com/osmosis-labs/sqs/tokens/usecase/pricing/coingecko"
)

var (
	zeroBigDec = osmomath.ZeroBigDec()
	oneBigDec  = osmomath.NewBigDec(1)
)

// MockCoingeckoPriceGetter is a mock implementation of CoingeckoPriceGetterFn
var DefaultMockCoingeckoPriceGetter coingeckopricing.CoingeckoPriceGetterFn = func(ctx context.Context, baseDenom string, coingeckoId string) (osmomath.BigDec, error) {
	if coingeckoId == "" {
		return zeroBigDec, nil
	} else {
		return oneBigDec, nil
	}
}
