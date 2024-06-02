package mocks

import (
	"fmt"

	"cosmossdk.io/math"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
)

type TokensPoolLiquidityHandlerMock struct {
	DenomScalingFactorMap map[string]math.LegacyDec

	PoolDenomMetadataMap domain.PoolDenomMetaDataMap
}

var _ mvc.TokensPoolLiquidityHandler = &TokensPoolLiquidityHandlerMock{}

type ScalingFactorNotFoundErr struct {
	Denom string
}

func (s ScalingFactorNotFoundErr) Error() string {
	return fmt.Sprintf("scaling factor not found for denom %s", s.Denom)
}

// GetChainScalingFactorByDenomMut implements mvc.TokensPoolLiquidityHandler.
func (t *TokensPoolLiquidityHandlerMock) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	scalingFactor, ok := t.DenomScalingFactorMap[denom]
	if !ok {
		return osmomath.Dec{}, ScalingFactorNotFoundErr{Denom: denom}
	}

	return scalingFactor, nil
}

// UpdatePoolDenomMetadata implements mvc.TokensPoolLiquidityHandler.
func (t *TokensPoolLiquidityHandlerMock) UpdatePoolDenomMetadata(tokensMetadata domain.PoolDenomMetaDataMap) {
	t.PoolDenomMetadataMap = tokensMetadata
}
