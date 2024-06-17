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

	// Data for Orderbook contract, must be present if and only if `IsOrderbook()` is true
	Orderbook *OrderbookData `json:"orderbook,omitempty"`
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
	name := "crates.io:transmuter"
	version := ">= 3.0.0"

	constraints, err := semver.NewConstraint(version)
	// this must never panic
	if err != nil {
		panic(err)
	}
	return model.ContractInfo.Matches(name, constraints)
}

func (model *CosmWasmPoolModel) IsOrderbook() bool {
	name := "crates.io:sumtree-orderbook"
	version := ">= 0.1.0"

	constraints, err := semver.NewConstraint(version)
	// this must never panic
	if err != nil {
		panic(err)
	}
	return model.ContractInfo.Matches(name, constraints)
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

type OrderbookDirection int

const (
	BID OrderbookDirection = 1
	ASK OrderbookDirection = -1
)

func (d *OrderbookDirection) String() string {
	switch *d {
	case BID:
		return "BID"
	case ASK:
		return "ASK"
	default:
		return "UNKNOWN"
	}
}

func (d *OrderbookDirection) Opposite() OrderbookDirection {
	switch *d {
	case BID:
		return ASK
	case ASK:
		return BID
	default:
		return 0
	}
}

// OrderbookData, since v1.0.0
type OrderbookData struct {
	QuoteDenom  string           `json:"quote_denom"`
	BaseDenom   string           `json:"base_denom"`
	NextBidTick int64            `json:"next_bid_tick"`
	NextAskTick int64            `json:"next_ask_tick"`
	Ticks       []TickIdAndState `json:"ticks"`
}

// Returns tick state index for the given ID
func (d *OrderbookData) GetTickIndexById(tickId int64) int {
	for i, tick := range d.Ticks {
		if tick.TickId == tickId {
			return i
		}
	}
	return -1
}

type TickValues struct {
	// Total Amount of Liquidity at tick (TAL)
	// - Every limit order placement increments this value.
	// - Every swap at this tick decrements this value.
	// - Every cancellation decrements this value.
	TotalAmountOfLiquidity osmomath.BigDec `json:"total_amount_of_liquidity"`
}

// Determines how much of a given amount can be filled by the current tick state (independent for each direction)
func (t *TickValues) GetFillableAmount(input osmomath.BigDec) osmomath.BigDec {
	if input.LT(t.TotalAmountOfLiquidity) {
		return input
	}
	return t.TotalAmountOfLiquidity
}

// Represents the state of a specific price tick in a liquidity pool.
//
// The state is split into two parts for the ask and bid directions.
type TickState struct {
	// Values for the ask direction of the tick
	AskValues TickValues `json:"ask_values"`
	// Values for the bid direction of the tick
	BidValues TickValues `json:"bid_values"`
}

// Returns the related values for a given direction on the current tick
func (s *TickState) GetTickValues(direction OrderbookDirection) (TickValues, error) {
	switch direction {
	case ASK:
		return s.AskValues, nil
	case BID:
		return s.BidValues, nil
	default:
		return TickValues{}, OrderbookPoolInvalidDirectionError{Direction: int64(direction)}
	}
}

type TickIdAndState struct {
	TickId    int64     `json:"tick_id"`
	TickState TickState `json:"tick_state"`
}
