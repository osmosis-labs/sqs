package sqsdomain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v23/x/poolmanager/types"
)

type RoutablePool interface {
	GetId() uint64

	GetType() poolmanagertypes.PoolType

	// IsGeneralizedCosmWasmPool returns true if this is a generalized cosmwasm pool.
	// Pools with such code ID are enabled in the router. For computing quotes or spot price,
	// they interact with the chain. Additionally, routes that contain such pools are disabled
	// in the router.
	IsGeneralizedCosmWasmPool() bool

	// GetCodeID returns the code ID of the pool if this is a CosmWasm pool.
	// If this is not a CosmWasm pool, it returns 0.
	GetCodeID() uint64

	GetPoolDenoms() []string

	GetTokenOutDenom() string

	CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error)

	CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error)
	ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin)

	// SetTokenOutDenom sets the token out denom on the routable pool.
	SetTokenOutDenom(tokenOutDenom string)

	GetTakerFee() osmomath.Dec

	GetSpreadFactor() osmomath.Dec

	String() string
}
