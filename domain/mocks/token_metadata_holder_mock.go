package mocks

import (
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type TokenMetadataHolderMock struct {
	MockMinPoolLiquidityCap               uint64
	MockMinPoolLiquidityCapError          error
	MockMinPoolEffectiveLiquidityCap      uint64
	MockMinPoolEffectiveLiquidityCapError error
	MockPoolDenomMetadata                 map[string]domain.PoolDenomMetaData
}

var _ mvc.TokenMetadataHolder = &TokenMetadataHolderMock{}

// GetMinPoolLiquidityCap implements mvc.TokenMetadataHolder.
func (t *TokenMetadataHolderMock) GetMinPoolLiquidityCap(denomA string, denomB string) (uint64, error) {
	return t.MockMinPoolLiquidityCap, t.MockMinPoolLiquidityCapError
}

// GetPoolDenomMetadata implements mvc.TokenMetadataHolder.
func (t *TokenMetadataHolderMock) GetPoolDenomMetadata(chainDenom string) (domain.PoolDenomMetaData, error) {
	poolDenomMetadata, ok := t.MockPoolDenomMetadata[chainDenom]
	if !ok {
		return domain.PoolDenomMetaData{}, domain.PoolDenomMetaDataNotPresentError{ChainDenom: chainDenom}
	}
	return poolDenomMetadata, nil
}

// GetMinPoolEffectiveLiquidityCap implements mvc.TokenMetadataHolder.
func (t *TokenMetadataHolderMock) GetMinPoolEffectiveLiquidityCap(denomA string, denomB string) (uint64, error) {
	return t.MockMinPoolEffectiveLiquidityCap, t.MockMinPoolEffectiveLiquidityCapError
}
