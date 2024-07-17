package passthroughdomain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

type PortfolioCategoryResult struct {
	TotalValueCap osmomath.Dec         `json:"total_value_cap"`
	Coins         []AccountCoinsResult `json:"coins"`
}

type PortfolioAssetsResult struct {
	TotalValueCap osmomath.Dec `json:"total_value_cap"`

	AccountCoinsResult []AccountCoinsResult `json:"account_coins_result"`
}

type AccountCoinsResult struct {
	Coin                sdk.Coin     `json:"coin"`
	CapitalizationValue osmomath.Dec `json:"cap_value"`
}
