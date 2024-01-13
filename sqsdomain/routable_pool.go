package sqsdomain

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/osmomath"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v21/x/poolmanager/types"
)

type RoutablePool interface {
	GetId() uint64

	GetType() poolmanagertypes.PoolType

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
