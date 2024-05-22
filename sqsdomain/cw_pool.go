package sqsdomain

import "github.com/osmosis-labs/osmosis/osmomath"

// CosmWasm contract info from [cw2 spec](https://github.com/CosmWasm/cw-minus/blob/main/packages/cw2/README.md)
type ContractInfo struct {
	Contract string `json:"contract"`
	Version  string `json:"version"`
}

// Check if the contract info matches the given contract and version
// The version can be a semver range
func (ci *ContractInfo) Matches(contract, version string) bool {
	return ci.Contract == contract && ci.Version == version
}

type CWPoolModel struct {
	ContractInfo ContractInfo `json:"contract_info"`
	Data         CWPoolData   `json:"data"`
}

type CWPoolData struct {
	AlloyTransmuter *AlloyTransmuterData `json:"alloy_transmuter,omitempty"`
}

func NewCWPoolModel(contract string, version string, data CWPoolData) *CWPoolModel {
	return &CWPoolModel{
		ContractInfo: ContractInfo{
			Contract: contract,
			Version:  version,
		},
		Data: data,
	}
}

func (model *CWPoolModel) IsAlloyTransmuter() bool {
	return model.ContractInfo.Matches("crates.io:transmuter", "3.0.0")
}

// === custom cw pool data ===

// Tranmuter Alloyed Data, since v3.0.0
// AssetConfigs is a list of denom and normalization factor pairs including the alloyed denom.
type AlloyTransmuterData struct {
	AlloyedDenom string                  `json:"alloyed_denom"`
	AssetConfigs []TransmuterAssetConfig `json:"asset_configs"`
}

type TransmuterAssetConfig struct {
	Denom               string       `json:"denom"`
	NormalizationFactor osmomath.Int `json:"normalization_factor"`
}
