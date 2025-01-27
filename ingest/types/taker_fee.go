package types

import "github.com/osmosis-labs/osmosis/osmomath"

// TakerFeeForPair represents the taker fee for a pair of tokens
type TakerFeeForPair struct {
	Denom0   string
	Denom1   string
	TakerFee osmomath.Dec
}
