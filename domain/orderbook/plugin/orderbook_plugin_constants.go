package orderbookplugindomain

import "github.com/osmosis-labs/osmosis/osmomath"

const (
	// BaseDenom is the base chain denom used for gas fees etc.
	BaseDenom = "uosmo"
)

var (
	// MinBalanceValueInUSDC is the minimum balance in USDC that has to be in the
	// orderbook pool to be considered for orderbook filling.
	MinBalanceValueInUSDC = osmomath.NewDec(10)
)
