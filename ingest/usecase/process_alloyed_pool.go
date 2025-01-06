package usecase

import (
	"fmt"
	"math/big"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
)

// processAlloyedPool processes the alloyed pool and computes the standard normalization factor and normalization scaling factors.
// Mutates the model with the computed values.
// Returns error if fails to computed either.
// CONTRACT: the caller checked that this is an alloyed pool
func processAlloyedPool(sqsModel *ingesttypes.SQSPool) error {
	if len(sqsModel.CosmWasmPoolModel.Data.AlloyTransmuter.AssetConfigs) == 0 {
		return fmt.Errorf("no asset configs found for alloyed pool")
	}

	cosmWasmModel := sqsModel.CosmWasmPoolModel

	// Compute the standard normalization factor
	standardNormalizationFactor, err := computeStandardNormalizationFactor(cosmWasmModel.Data.AlloyTransmuter.AssetConfigs)
	if err != nil {
		return err
	}

	// Compute the scaling factor normalization factors from the standard normalization factor for each asset
	normalizationScalingFactors, err := computeNormalizationScalingFactors(standardNormalizationFactor, cosmWasmModel.Data.AlloyTransmuter.AssetConfigs)
	if err != nil {
		return err
	}

	// Update the precomputed data in the model
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
// Returns error if one of the asset normalization factors is nil or zero.
// Returns error if the standard normalization factor is nil or zero.
// Returns error if asset scaling factor truncates to zero.
func computeNormalizationScalingFactors(standardNormalizationFactor osmomath.Int, assetConfigs []cosmwasmpool.TransmuterAssetConfig) (map[string]osmomath.Int, error) {
	if standardNormalizationFactor.IsNil() || standardNormalizationFactor.IsZero() {
		return nil, fmt.Errorf("standard normalization factor is nil or zero")
	}

	scalingFactors := make(map[string]osmomath.Int, len(assetConfigs))
	for i := 0; i < len(assetConfigs); i++ {
		assetConfig := assetConfigs[i]
		assetNormalizationFactor := assetConfig.NormalizationFactor
		if assetNormalizationFactor.IsNil() || assetNormalizationFactor.IsZero() {
			return nil, fmt.Errorf("normalization factor is nil or zero for asset %s", assetConfig.Denom)
		}

		assetScalingFactor := standardNormalizationFactor.Quo(assetNormalizationFactor)

		if assetScalingFactor.IsZero() {
			return nil, fmt.Errorf("scaling factor truncated to zero for asset %s", assetConfig.Denom)
		}

		scalingFactors[assetConfig.Denom] = assetScalingFactor
	}
	return scalingFactors, nil
}

// Lcm calculates the least common multiple of two big.Int values.
func Lcm(a *big.Int, b *big.Int) *big.Int {
	return new(big.Int).Div(new(big.Int).Mul(a, b), new(big.Int).GCD(nil, nil, a, b))
}
