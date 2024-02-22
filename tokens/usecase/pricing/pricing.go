package pricing

import "github.com/osmosis-labs/osmosis/osmomath"

type PricingStrategy interface {
	GetPrices(baseDenoms []string, quoteDenoms []string) map[string]map[string]osmomath.BigDec
}
