package sqsdomain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// Note that pool types must match the on-chain definition.
// We avoid importing this from chain and redefine to minimize
// the number of dependecies:
// https://github.com/osmosis-labs/osmosis/blob/891866b619754dddf13b871b394140bfa17c5025/x/poolmanager/types/module_route.pb.go#L29-L41
type PoolType int32

const (
	// Balancer is the standard xy=k curve. Its pool model is defined in x/gamm.
	Balancer PoolType = 0
	// Stableswap is the Solidly cfmm stable swap curve. Its pool model is defined
	// in x/gamm.
	Stableswap PoolType = 1
	// Concentrated is the pool model specific to concentrated liquidity. It is
	// defined in x/concentrated-liquidity.
	Concentrated PoolType = 2
	// CosmWasm is the pool model specific to CosmWasm. It is defined in
	// x/cosmwasmpool.
	CosmWasm PoolType = 3

	// Convinience defintion to reflect the max supported pool type value.
	MaxSupportedType = CosmWasm
)

type RoutablePool interface {
	GetId() uint64

	GetType() PoolType

	// IsGeneralizedCosmWasmPool returns true if this is a generalized cosmwasm pool.
	// Pools with such code ID are enabled in the router. For computing quotes or spot price,
	// they interact with the chain. Additionally, routes that contain such pools are disabled
	// in the router.
	IsGeneralizedCosmWasmPool() bool

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
