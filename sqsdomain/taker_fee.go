package sqsdomain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// DenomPair encapsulates a pair of denoms.
// The order of the denoms ius that Denom0 precedes
// Denom1 lexicographically.
type DenomPair struct {
	Denom0 string
	Denom1 string
}

// TakerFeeMap is a map of DenomPair to taker fee.
// It sorts the denoms lexicographically before looking up the taker fee.
type TakerFeeMap map[DenomPair]osmomath.Dec

var _ json.Marshaler = &TakerFeeMap{}
var _ json.Unmarshaler = &TakerFeeMap{}

// MarshalJSON implements json.Marshaler.
func (tfm TakerFeeMap) MarshalJSON() ([]byte, error) {
	serializedMap := map[string]osmomath.Dec{}
	for key, value := range tfm {
		// Convert DenomPair to a string representation
		keyString := fmt.Sprintf("%s|%s", key.Denom0, key.Denom1)
		serializedMap[keyString] = value
	}

	return json.Marshal(serializedMap)
}

// UnmarshalJSON implements json.Unmarshaler.
func (tfm TakerFeeMap) UnmarshalJSON(data []byte) error {
	var serializedMap map[string]osmomath.Dec
	if err := json.Unmarshal(data, &serializedMap); err != nil {
		return err
	}

	// Convert string keys back to DenomPair
	for keyString, value := range serializedMap {
		parts := strings.Split(keyString, "|")
		if len(parts) != 2 {
			return fmt.Errorf("invalid key format: %s", keyString)
		}
		denomPair := DenomPair{Denom0: parts[0], Denom1: parts[1]}
		(tfm)[denomPair] = value
	}

	return nil
}

// Has returns true if the taker fee for the given denoms is found.
// It sorts the denoms lexicographically before looking up the taker fee.
func (tfm TakerFeeMap) Has(denom0, denom1 string) bool {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	_, found := tfm[DenomPair{Denom0: denom0, Denom1: denom1}]
	return found
}

// GetTakerFee returns the taker fee for the given denoms.
// It sorts the denoms lexicographically before looking up the taker fee.
// Returns error if the taker fee is not found.
func (tfm TakerFeeMap) GetTakerFee(denom0, denom1 string) osmomath.Dec {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	takerFee, found := tfm[DenomPair{Denom0: denom0, Denom1: denom1}]

	if !found {
		return DefaultTakerFee
	}

	return takerFee
}

// SetTakerFee sets the taker fee for the given denoms.
// It sorts the denoms lexicographically before setting the taker fee.
func (tfm TakerFeeMap) SetTakerFee(denom0, denom1 string, takerFee osmomath.Dec) {
	// Ensure increasing lexicographic order.
	if denom1 < denom0 {
		denom0, denom1 = denom1, denom0
	}

	tfm[DenomPair{Denom0: denom0, Denom1: denom1}] = takerFee
}

// TakerFeeForPair represents the taker fee for a pair of tokens
type TakerFeeForPair struct {
	Denom0   string
	Denom1   string
	TakerFee osmomath.Dec
}

var DefaultTakerFee = osmomath.MustNewDecFromStr("0.001000000000000000")
