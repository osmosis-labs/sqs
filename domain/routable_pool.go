package domain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

// SQSPoolType is an enum that represents the type of SQS pool.
// Each definition in the enum corresponds to a specific pool type implementation in SQS.
// These are located in routes/usecase/pools.
type SQSPoolType int

const (
	// Mock is a mock pool type for testing purposes.
	Mock = -2
	// Result is a result pool type for returning to clients.
	Result = -1
	// Balancer is a Balancer pool type.
	Balancer SQSPoolType = iota
	// StableSwap is a StableSwap pool type.
	StableSwap
	// Concentrated is a Concentrated pool type.
	Concentrated
	// TransmuterV1 is a TransmuterV1 pool type.
	TransmuterV1
	// GeneralizedCosmWasm is a GeneralizedCosmWasm pool type.
	GeneralizedCosmWasm
	// AlloyedTransmuter is an AlloyedTransmuter pool type.
	AlloyedTransmuter
)

// RoutablePool is an interface that represents a pool that can be routed over.
type RoutablePool interface {
	GetId() uint64

	GetType() poolmanagertypes.PoolType

	// GetSQSType returns the SQS pool type.
	// Each definition in the SQSPoolType enum corresponds to a specific pool type
	// implementation in SQS.
	GetSQSType() SQSPoolType

	// GetCodeID returns the code ID of the pool if this is a CosmWasm pool.
	// If this is not a CosmWasm pool, it returns 0.
	GetCodeID() uint64

	GetPoolDenoms() []string

	GetTokenOutDenom() string

	CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)

	CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error)
	ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin)

	GetTakerFee() osmomath.Dec

	GetSpreadFactor() osmomath.Dec

	String() string
}
