package passthroughdomain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// PortfolioAssetsResult represents the total value of the assets in the portfolio.
type PortfolioAssetsResult struct {
	// TotalValueCap represents the total value of the assets in the portfolio.
	// includes capitalization of user balances, value in locks, bonding or unbonding
	// as well as the concentrated positions.
	TotalValueCap osmomath.Dec `json:"total_value_cap"`
	// AccountCoinsResult represents coins only from user balances (contrary to TotalValueCap).
	AccountCoinsResult []AccountCoinsResult `json:"account_coins_result"`
}

// AccountCoinsResult represents the coin balance as well as its capitalization value.
type AccountCoinsResult struct {
	Coin                sdk.Coin     `json:"coin"`
	CapitalizationValue osmomath.Dec `json:"cap_value"`
}
