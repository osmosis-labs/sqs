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
