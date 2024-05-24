package mocks

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	coingeckopricing "github.com/osmosis-labs/sqs/tokens/usecase/pricing/coingecko"
)

const (
	ATOM_COINGECKO_ID = "cosmos"
)

var (
	ZeroBigDec = osmomath.ZeroBigDec()
	OneBigDec  = osmomath.NewBigDec(1)
	AtomPrice  = osmomath.NewBigDec(5)
)

// MockCoingeckoPriceGetter is a mock implementation of CoingeckoPriceGetterFn
var DefaultMockCoingeckoPriceGetter coingeckopricing.CoingeckoPriceGetterFn = func(ctx context.Context, baseDenom string, coingeckoId string) (osmomath.BigDec, error) {
	if coingeckoId == "" {
		return ZeroBigDec, nil
	} else if coingeckoId == ATOM_COINGECKO_ID {
		return AtomPrice, nil
	} else {
		return OneBigDec, nil
	}
}
