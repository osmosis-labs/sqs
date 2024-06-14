package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// TokensUsecaseMock is a mock implementation of the TokensUsecase interface
type TokensUsecaseMock struct {
	LoadTokensFunc                       func(tokenMetadataByChainDenom map[string]domain.Token)
	GetMetadataByChainDenomFunc          func(denom string) (domain.Token, error)
	GetFullTokenMetadataFunc             func() (map[string]domain.Token, error)
	GetChainDenomFunc                    func(humanDenom string) (string, error)
	GetSpotPriceScalingFactorByDenomFunc func(baseDenom, quoteDenom string) (osmomath.Dec, error)
	GetPricesFunc                        func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error)
	RegisterPricingStrategyFunc          func(source domain.PricingSourceType, strategy domain.PricingSource)
	IsValidChainDenomFunc                func(chainDenom string) bool
	IsValidPricingSourceFunc             func(pricingSource int) bool
	GetCoingeckoIdByChainDenomFunc       func(chainDenom string) (string, error)
}

func (m *TokensUsecaseMock) LoadTokens(tokenMetadataByChainDenom map[string]domain.Token) {
	if m.LoadTokensFunc != nil {
		m.LoadTokensFunc(tokenMetadataByChainDenom)
	}
}

func (m *TokensUsecaseMock) GetMetadataByChainDenom(denom string) (domain.Token, error) {
	if m.GetMetadataByChainDenomFunc != nil {
		return m.GetMetadataByChainDenomFunc(denom)
	}
	return domain.Token{}, nil
}

func (m *TokensUsecaseMock) GetFullTokenMetadata() (map[string]domain.Token, error) {
	if m.GetFullTokenMetadataFunc != nil {
		return m.GetFullTokenMetadataFunc()
	}
	return nil, nil
}

func (m *TokensUsecaseMock) GetChainDenom(humanDenom string) (string, error) {
	if m.GetChainDenomFunc != nil {
		return m.GetChainDenomFunc(humanDenom)
	}
	return "", nil
}

func (m *TokensUsecaseMock) GetSpotPriceScalingFactorByDenom(baseDenom, quoteDenom string) (osmomath.Dec, error) {
	if m.GetSpotPriceScalingFactorByDenomFunc != nil {
		return m.GetSpotPriceScalingFactorByDenomFunc(baseDenom, quoteDenom)
	}
	return osmomath.Dec{}, nil
}

func (m *TokensUsecaseMock) GetPrices(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error) {
	if m.GetPricesFunc != nil {
		return m.GetPricesFunc(ctx, baseDenoms, quoteDenoms, pricingSourceType, opts...)
	}
	return domain.PricesResult{}, nil
}

func (m *TokensUsecaseMock) RegisterPricingStrategy(source domain.PricingSourceType, strategy domain.PricingSource) {
	if m.RegisterPricingStrategyFunc != nil {
		m.RegisterPricingStrategyFunc(source, strategy)
	}
}

func (m *TokensUsecaseMock) IsValidChainDenom(chainDenom string) bool {
	if m.IsValidChainDenomFunc != nil {
		return m.IsValidChainDenomFunc(chainDenom)
	}
	return false
}

func (m *TokensUsecaseMock) IsValidPricingSource(pricingSource int) bool {
	if m.IsValidPricingSourceFunc != nil {
		return m.IsValidPricingSourceFunc(pricingSource)
	}
	return false
}

func (m *TokensUsecaseMock) GetCoingeckoIdByChainDenom(chainDenom string) (string, error) {
	if m.GetCoingeckoIdByChainDenomFunc != nil {
		return m.GetCoingeckoIdByChainDenomFunc(chainDenom)
	}
	return "", nil
}
