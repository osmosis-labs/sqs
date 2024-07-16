package passthroughdomain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

type AccountCoinsResult struct {
	Coin                sdk.Coin     `json:"coin"`
	CapitalizationValue osmomath.Int `json:"cap_value"`
}

