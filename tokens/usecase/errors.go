package usecase

import "fmt"

// ChainDenomNotFoundInChainRegistryError represents error type for when
// chain denom not found in chain registry.
type ChainDenomNotFoundInChainRegistryError struct{}

// Error implements the error interface.
func (e ChainDenomNotFoundInChainRegistryError) Error() string {
	return "chain denom not found in chain registry"
}

// ChainDenomNotValidTypeError represents error type for when coingecko id is not a valid type.
type CoingeckoIDNotValidTypeError struct {
	CoingeckoID any
	Denom       string
}

// Error implements the error interface.
func (e CoingeckoIDNotValidTypeError) Error() string {
	return fmt.Sprintf("coingecko id ( %v ) for chain denom (%v) is not of type string", e.CoingeckoID, e.Denom)
}

// ChainDenomForHumanDenomNotFoundError represents error type for when a chain denom
// for a human denom is not found.
type ChainDenomForHumanDenomNotFoundError struct {
	ChainDenom string
}

// Error implements the error interface.
func (e ChainDenomForHumanDenomNotFoundError) Error() string {
	return fmt.Sprintf("chain denom for human denom (%s) is not found", e.ChainDenom)
}

// MetadataForChainDenomNotFoundError  represents error type for when metadata for a chain denom
// is not found.
type MetadataForChainDenomNotFoundError struct {
	ChainDenom string
}

// Error implements the error interface.
func (e MetadataForChainDenomNotFoundError) Error() string {
	return fmt.Sprintf("metadata for denom (%s) is not found", e.ChainDenom)
}

// HumanDenomForChainDenomNotFoundError represents error type for when a metadata
// for a chain denom is not a valid type.
type MetadataForChainDenomNotValidTypeError struct {
	ChainDenom string
}

// Error implements the error interface.
func (e MetadataForChainDenomNotValidTypeError) Error() string {
	return fmt.Sprintf("metadata for denom (%v) is not of type domain.Token", e.ChainDenom)
}

// HumanDenomNotValidTypeError represents error type for when human denom is not a valid type
type HumanDenomNotValidTypeError struct {
	HumanDenom any
}

// Error implements the error interface.
func (e HumanDenomNotValidTypeError) Error() string {
	return fmt.Sprintf("human denom (%v) is not of type string", e.HumanDenom)
}

// DenomForHumanDenomNotFoundError represents error type for when a denom is not a valid type
type DenomNotValidTypeError struct {
	Denom any
}

// Error implements the error interface.
func (e DenomNotValidTypeError) Error() string {
	return fmt.Sprintf("denom (%v) is not of type string", e.Denom)
}

// TokenForDenomNotFoundError represents error type for when a token is not a valid type
type TokenNotValidTypeError struct {
	Token any
}

// Error implements the error interface.
func (e TokenNotValidTypeError) Error() string {
	return fmt.Sprintf("token (%v) is not of type domain.Token", e.Token)
}

// ScalingFactorForPrecisionNotFoundError represents error type for when a scaling factor
// for denom precision is not found.
type ScalingFactorForPrecisionNotFoundError struct {
	Precision int
	Denom     string
}

// Error implements the error interface.
func (e ScalingFactorForPrecisionNotFoundError) Error() string {
	return fmt.Sprintf("scaling factor for precision (%d) and denom (%s) not found", e.Precision, e.Denom)
}
