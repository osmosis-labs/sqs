package cosmwasmpool

import (
	"github.com/Masterminds/semver"
	"github.com/osmosis-labs/osmosis/osmomath"
)

func (model *CosmWasmPoolModel) IsAlloyTransmuter() bool {
	name := "crates.io:transmuter"
	version := ">= 3.0.0"

	constraints, err := semver.NewConstraint(version)
	// this must never panic
	if err != nil {
		panic(err)
	}
	return model.ContractInfo.Matches(name, constraints)
}

// Tranmuter Alloyed Data, since v3.0.0
// AssetConfigs is a list of denom and normalization factor pairs including the alloyed denom.
type AlloyTransmuterData struct {
	AlloyedDenom string                  `json:"alloyed_denom"`
	AssetConfigs []TransmuterAssetConfig `json:"asset_configs"`
}

// Configuration for each asset in the transmuter pool
type TransmuterAssetConfig struct {
	// Denom of the asset
	Denom string `json:"denom"`

	// Normalization factor for the asset.
	// [more info](https://github.com/osmosis-labs/transmuter/tree/v3.0.0?tab=readme-ov-file#normalization-factors)
	NormalizationFactor osmomath.Int `json:"normalization_factor"`
}
