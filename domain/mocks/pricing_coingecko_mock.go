package mocks

import (
	"context"
	"fmt"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

const (
	ATOM_COINGECKO_ID = "cosmos"
)

var (
	NilBigDec = osmomath.BigDec{}
	OneBigDec = osmomath.NewBigDec(1)
	AtomPrice = osmomath.NewBigDec(5)
)

// CoingeckoPricingSourceMock is a Coingecko pricing source that prices every token with a
// Coingecko id without a network request: ATOM_COINGECKO_ID at AtomPrice and any other at OneBigDec.
type CoingeckoPricingSourceMock struct {
	Tokens mvc.TokensUsecase
}

var _ domain.PricingSource = CoingeckoPricingSourceMock{}

// GetPrice implements domain.PricingSource.
func (m CoingeckoPricingSourceMock) GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	coingeckoId, err := m.Tokens.GetCoingeckoIdByChainDenom(baseDenom)
	if err != nil {
		return NilBigDec, nil, err
	}
	switch coingeckoId {
	case "":
		return NilBigDec, nil, domain.NoPriceError{Err: fmt.Errorf("coingecko ID is empty for base (%s)", baseDenom)}
	case ATOM_COINGECKO_ID:
		return AtomPrice, nil, nil
	default:
		return OneBigDec, nil, nil
	}
}

// InitializeCache implements domain.PricingSource.
func (CoingeckoPricingSourceMock) InitializeCache(*cache.Cache) {}

// GetFallbackStrategy implements domain.PricingSource.
func (CoingeckoPricingSourceMock) GetFallbackStrategy(string) domain.PricingSourceType {
	return domain.NoneSourceType
}
