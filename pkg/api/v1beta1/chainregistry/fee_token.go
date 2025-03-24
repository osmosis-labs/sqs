package chainregistry

// FeeTokens is a list of FeeToken.
type FeeTokens []*FeeToken

// GetByDenom returns the FeeToken by the given denom.
func (t FeeTokens) GetByDenom(denom string) *FeeToken {
	for _, token := range t {
		if token.Denom == denom {
			return token
		}
	}
	return nil
}
