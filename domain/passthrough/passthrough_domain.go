package passthroughdomain

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
)

// PortfolioAssetsCategoryResult represents the total value of the assets in the portfolio.
type PortfolioAssetsResult struct {
	// Categories represents the breakdown of assets in the portfolio by categories.
	Categories map[string]PortfolioAssetsCategoryResult `json:"categories"`
	// TotalValueCap represents the total value of the assets in the portfolio.
	// includes capitalization of user balances, value in locks, bonding or unbonding
	// as well as the concentrated positions.
	TotalValueCap osmomath.Dec `json:"total_value_cap"`
}

type PortfolioAssetsResult2 struct {
	Categories map[string]PortfolioAssetsCategoryResult `json:"categories"`
}

// PortfolioAssetsCategoryResult represents the total value of the assets in the portfolio.
type PortfolioAssetsCategoryResult struct {
	// Capitalization represents the total value of the assets in the portfolio.
	// includes capitalization of user balances, value in locks, bonding or unbonding
	// as well as the concentrated positions.
	Capitalization osmomath.Dec `json:"capitalization"`
	// AccountCoinsResult represents coins only from user balances (contrary to TotalValueCap).
	AccountCoinsResult []AccountCoinsResult `json:"account_coins_result,omitempty"`

	IsBestEffort bool `json:"is_best_effort"`
}

// AccountCoinsResult represents the coin balance as well as its capitalization value.
type AccountCoinsResult struct {
	Coin                sdk.Coin     `json:"coin"`
	CapitalizationValue osmomath.Dec `json:"cap_value"`
}
