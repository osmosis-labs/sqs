package mocks

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// TokensUsecaseMock is a mock implementation of the TokensUsecase interface
type TokensUsecaseMock struct {
	UpdatePoolDenomMetadataFunc          func(tokensMetadata domain.PoolDenomMetaDataMap)
	LoadTokensFunc                       func(tokenMetadataByChainDenom map[string]domain.Token)
	GetMetadataByChainDenomFunc          func(denom string) (domain.Token, error)
	GetFullTokenMetadataFunc             func() (map[string]domain.Token, error)
	GetChainDenomFunc                    func(humanDenom string) (string, error)
	GetChainScalingFactorByDenomMutFunc  func(denom string) (osmomath.Dec, error)
	GetSpotPriceScalingFactorByDenomFunc func(baseDenom, quoteDenom string) (osmomath.Dec, error)
	GetPricesFunc                        func(ctx context.Context, baseDenoms []string, quoteDenoms []string, pricingSourceType domain.PricingSourceType, opts ...domain.PricingOption) (domain.PricesResult, error)
	GetMinPoolLiquidityCapFunc           func(denomA, denomB string) (uint64, error)
	GetPoolDenomMetadataFunc             func(chainDenom string) (domain.PoolDenomMetaData, error)
	GetPoolLiquidityCapFunc              func(chainDenom string) (osmomath.Int, error)
	GetPoolDenomsMetadataFunc            func(chainDenoms []string) domain.PoolDenomMetaDataMap
	GetFullPoolDenomMetadataFunc         func() domain.PoolDenomMetaDataMap
	RegisterPricingStrategyFunc          func(source domain.PricingSourceType, strategy domain.PricingSource)
	IsValidChainDenomFunc                func(chainDenom string) bool
	IsValidPricingSourceFunc             func(pricingSource int) bool
	GetCoingeckoIdByChainDenomFunc       func(chainDenom string) (string, error)
	UpdateAssetsAtHeightIntervalFunc     func(height uint64)
}

func (m *TokensUsecaseMock) UpdatePoolDenomMetadata(tokensMetadata domain.PoolDenomMetaDataMap) {
	if m.UpdatePoolDenomMetadataFunc != nil {
		m.UpdatePoolDenomMetadataFunc(tokensMetadata)
	}
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

func (m *TokensUsecaseMock) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	if m.GetChainScalingFactorByDenomMutFunc != nil {
		return m.GetChainScalingFactorByDenomMutFunc(denom)
	}
	return osmomath.Dec{}, nil
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

func (m *TokensUsecaseMock) GetMinPoolLiquidityCap(denomA, denomB string) (uint64, error) {
	if m.GetMinPoolLiquidityCapFunc != nil {
		return m.GetMinPoolLiquidityCapFunc(denomA, denomB)
	}
	return 0, nil
}

func (m *TokensUsecaseMock) GetPoolDenomMetadata(chainDenom string) (domain.PoolDenomMetaData, error) {
	if m.GetPoolDenomMetadataFunc != nil {
		return m.GetPoolDenomMetadataFunc(chainDenom)
	}
	return domain.PoolDenomMetaData{}, nil
}

func (m *TokensUsecaseMock) GetPoolLiquidityCap(chainDenom string) (osmomath.Int, error) {
	if m.GetPoolLiquidityCapFunc != nil {
		return m.GetPoolLiquidityCapFunc(chainDenom)
	}
	return osmomath.Int{}, nil
}

func (m *TokensUsecaseMock) GetPoolDenomsMetadata(chainDenoms []string) domain.PoolDenomMetaDataMap {
	if m.GetPoolDenomsMetadataFunc != nil {
		return m.GetPoolDenomsMetadataFunc(chainDenoms)
	}
	return domain.PoolDenomMetaDataMap{}
}

func (m *TokensUsecaseMock) GetFullPoolDenomMetadata() domain.PoolDenomMetaDataMap {
	if m.GetFullPoolDenomMetadataFunc != nil {
		return m.GetFullPoolDenomMetadataFunc()
	}
	return domain.PoolDenomMetaDataMap{}
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

func (m *TokensUsecaseMock) UpdateAssetsAtHeightInterval(height uint64) {
	if m.UpdateAssetsAtHeightIntervalFunc != nil {
		m.UpdateAssetsAtHeightIntervalFunc(height)
	}
}
