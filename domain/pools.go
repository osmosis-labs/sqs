package domain

import (
	"context"

	poolmanagertypes "github.com/osmosis-labs/osmosis/v20/x/poolmanager/types"
)

// PoolI represents a generalized Pool interface.
type PoolI interface {
	GetId() uint64
	// GetType returns the type of the pool (Balancer, Stableswap, Concentrated, etc.)
	GetType() poolmanagertypes.PoolType
}

// CFMMPoolI represents a constant function market maker pool interface
type CFMMPoolI interface {
	PoolI
}

// ConcentratedPoolI represents a concentrated liquidity pool inteface.
type ConcentratedPoolI interface {
	PoolI
}

// CosmWasmPoolI represents a cosm wasm pool interface.
type CosmWasmPoolI interface {
	PoolI
}

// PoolsUsecase represent the pool's usecases
type PoolsUsecase interface {
	GetAllPools(ctx context.Context) ([]PoolI, error)
}

// PoolsRepository represent the pool's repository contract
type PoolsRepository interface {
	// GetAllConcentrated returns concentrated pools sorted by ID.
	GetAllConcentrated(context.Context) ([]ConcentratedPoolI, error)
	// GetAllCFMM returns CFMM pools sorted by ID.
	GetAllCFMM(context.Context) ([]CFMMPoolI, error)
	// GetAllCosmWasm returns CosmWasm pools sorted by ID.
	GetAllCosmWasm(context.Context) ([]CosmWasmPoolI, error)

	StoreConcentrated(context.Context, []ConcentratedPoolI) error
	StoreCFMM(context.Context, []CFMMPoolI) error
	StoreCosmWasm(context.Context, []CosmWasmPoolI) error
}
