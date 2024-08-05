package usecase

import (
	"fmt"
	"math/big"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
)

// CONTRACT: the caller checked that this is an alloyed pool
func processAlloyedPool(sqsModel *sqsdomain.SQSPool) error {
	if len(sqsModel.CosmWasmPoolModel.Data.AlloyTransmuter.AssetConfigs) == 0 {
		return fmt.Errorf("no asset configs found for alloyed pool")
	}

	cosmWasmModel := sqsModel.CosmWasmPoolModel

	standardNormalizationFactor, err := computeStandardNormalizationFactor(cosmWasmModel.Data.AlloyTransmuter.AssetConfigs)
	if err != nil {
		return err
	}

	normalizationScalingFactors := computeNormalizationScalingFactors(standardNormalizationFactor, cosmWasmModel.Data.AlloyTransmuter.AssetConfigs)

	cosmWasmModel.Data.AlloyTransmuter.PreComputedData.StdNormFactor = standardNormalizationFactor

	cosmWasmModel.Data.AlloyTransmuter.PreComputedData.NormalizationScalingFactors = normalizationScalingFactors

	return nil
}

// computeStandardNormalizationFactor computes the standard normalization factor for the pool.
// Returns error if one of the asset normalization factors is nil or zero.
func computeStandardNormalizationFactor(assetConfigs []cosmwasmpool.TransmuterAssetConfig) (osmomath.Int, error) {
	result := osmomath.OneInt().BigIntMut()
	for i := 0; i < len(assetConfigs); i++ {
		normFactor := assetConfigs[i].NormalizationFactor
		if normFactor.IsNil() || normFactor.IsZero() {
			return osmomath.Int{}, fmt.Errorf("normalization factor is nil or zero for asset %s", assetConfigs[i])
		}

		currentNormFactor := assetConfigs[i].NormalizationFactor.BigInt()
		result = Lcm(result, currentNormFactor)
	}
	return osmomath.NewIntFromBigInt(result), nil
}

// computeNormalizationScalingFactors computes the normalization scaling factors for each denom in the asset config
// using the standard normalization factor.
func computeNormalizationScalingFactors(standardNormalizationFactor osmomath.Int, assetConfigs []cosmwasmpool.TransmuterAssetConfig) []osmomath.Int {
	scalingFactors := make([]osmomath.Int, len(assetConfigs))
	for i := 0; i < len(assetConfigs); i++ {
		scalingFactors[i] = standardNormalizationFactor.Quo(assetConfigs[i].NormalizationFactor)
	}
	return scalingFactors
}

// Lcm calculates the least common multiple of two big.Int values.
func Lcm(a *big.Int, b *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(a, b), new(big.Int).GCD(nil, nil, a, b))
}
