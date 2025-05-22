package mocks

import "github.com/osmosis-labs/sqs/domain/mvc"

type TokenMetadataHolderMock struct {
	MockMinPoolLiquidityCap               uint64
	MockMinPoolLiquidityCapError          error
	MockMinPoolEffectiveLiquidityCap      uint64
	MockMinPoolEffectiveLiquidityCapError error
}

var _ mvc.TokenMetadataHolder = &TokenMetadataHolderMock{}

// GetMinPoolLiquidityCap implements mvc.TokenMetadataHolder.
func (t *TokenMetadataHolderMock) GetMinPoolLiquidityCap(denomA string, denomB string) (uint64, error) {
	return t.MockMinPoolLiquidityCap, t.MockMinPoolLiquidityCapError
}

// GetMinPoolEffectiveLiquidityCap implements mvc.TokenMetadataHolder.
func (t *TokenMetadataHolderMock) GetMinPoolEffectiveLiquidityCap(denomA string, denomB string) (uint64, error) {
	return t.MockMinPoolEffectiveLiquidityCap, t.MockMinPoolEffectiveLiquidityCapError
}
