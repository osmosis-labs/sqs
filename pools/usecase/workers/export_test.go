package workers

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

func ComputeCoinTVL(coin sdk.Coin, baseDenomPriceData DenomPriceInfo) (osmomath.Dec, error) {
	return computeCoinTVL(coin, baseDenomPriceData)
}
