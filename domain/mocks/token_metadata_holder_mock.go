package mocks

import "github.com/osmosis-labs/sqs/domain/mvc"

type TokenMetadataHolderMock struct {
	MockMinPoolLiquidityCap      uint64
	MockMinPoolLiquidityCapError error
}

var _ mvc.TokenMetadataHolder = &TokenMetadataHolderMock{}

// GetMinPoolLiquidityCap implements mvc.TokenMetadataHolder.
func (t *TokenMetadataHolderMock) GetMinPoolLiquidityCap(denomA string, denomB string) (uint64, error) {
	return t.MockMinPoolLiquidityCap, t.MockMinPoolLiquidityCapError
}
