package sqsdomain

import (
	"github.com/Masterminds/semver"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// CosmWasm contract info from [cw2 spec](https://github.com/CosmWasm/cw-minus/blob/main/packages/cw2/README.md)
type ContractInfo struct {
	Contract string `json:"contract"`
	Version  string `json:"version"`
}

const (
	AlloyTranmuterName        = "crates.io:transmuter"
	AlloyTransmuterMinVersion = "3.0.0"

	alloyTransmuterMinVersionStr = ">= " + AlloyTransmuterMinVersion
)

// Check if the contract info matches the given contract and version constrains
func (ci *ContractInfo) Matches(contract string, versionConstrains *semver.Constraints) bool {
	version, err := semver.NewVersion(ci.Version)
	validSemver := err == nil

	// matches only if:
	// - semver is valid
	// - contract matches
	// - version constrains matches
	return validSemver && (ci.Contract == contract && versionConstrains.Check(version))
}

// CosmWasmPoolModel is a model for the pool data of a CosmWasm pool
// It includes the contract info and the pool data
// The CWPoolData works like a tagged union to hold different types of data
// depending on the contract and its version
type CosmWasmPoolModel struct {
	ContractInfo ContractInfo `json:"contract_info"`
	Data         CWPoolData   `json:"data"`
}

// CWPoolData is the custom data for each type of CosmWasm pool
// This struct is intended to work like tagged union in other languages
// so that it can hold different types of data depending on the contract
type CWPoolData struct {
	// Data for AlloyTransmuter contract, must be present if and only if `IsAlloyTransmuter()` is true
	AlloyTransmuter *AlloyTransmuterData `json:"alloy_transmuter,omitempty"`
}

func NewCWPoolModel(contract string, version string, data CWPoolData) *CosmWasmPoolModel {
	return &CosmWasmPoolModel{
		ContractInfo: ContractInfo{
			Contract: contract,
			Version:  version,
		},
		Data: data,
	}
}

func (model *CosmWasmPoolModel) IsAlloyTransmuter() bool {
	constraints, err := semver.NewConstraint(alloyTransmuterMinVersionStr)
	// this must never panic
	if err != nil {
		panic(err)
	}
	return model.ContractInfo.Matches(AlloyTranmuterName, constraints)
}

// === custom cw pool data ===

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
