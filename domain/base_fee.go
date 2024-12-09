package domain

import "github.com/osmosis-labs/osmosis/osmomath"

// BaseFee holds the denom and current base fee
type BaseFee struct {
	Denom      string
	CurrentFee osmomath.Dec
}
