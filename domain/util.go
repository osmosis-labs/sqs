package domain

// ValidateInputDenoms returns nil of two denoms are valid, otherwise an error.
// This is to be used as a parameter validation for queries.
// For example, token in denom must not equal token out denom for quotes.
func ValidateInputDenoms(denomA, denomB string) error {
	if denomA == denomB {
		return SameDenomError{
			DenomA: denomA,
			DenomB: denomB,
		}
	}

	return nil
}

// Generic function to extract keys from any map.
func KeysFromMap[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m)) // Pre-allocate slice with capacity equal to map size
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}
