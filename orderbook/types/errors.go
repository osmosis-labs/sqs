package types

import "fmt"

// TickForOrderbookNotFoundError represents an error when a tick is not found for a given orderbook.
type TickForOrderbookNotFoundError struct {
	OrderbookAddress string
	TickID           int64
}

// Error implements the error interface.
func (e TickForOrderbookNotFoundError) Error() string {
	return fmt.Sprintf("tick not found %s, %d", e.OrderbookAddress, e.TickID)
}

// ParsingQuantityError represents an error that occurs while parsing the quantity field.
type ParsingQuantityError struct {
	Quantity string
	Err      error
}

// Error implements the error interface.
func (e ParsingQuantityError) Error() string {
	return fmt.Sprintf("error parsing quantity %s: %v", e.Quantity, e.Err)
}

// ParsingPlacedQuantityError represents an error that occurs while parsing the placed quantity field.
type ParsingPlacedQuantityError struct {
	PlacedQuantity string
	Err            error
}

// Error implements the error interface.
func (e ParsingPlacedQuantityError) Error() string {
	return fmt.Sprintf("error parsing placed quantity %s: %v", e.PlacedQuantity, e.Err)
}

// InvalidPlacedQuantityError represents an error when the placed quantity is invalid.
type InvalidPlacedQuantityError struct {
	PlacedQuantity int64
}

// Error implements the error interface.
func (e InvalidPlacedQuantityError) Error() string {
	return fmt.Sprintf("placed quantity is 0 or negative: %d", e.PlacedQuantity)
}

// GettingSpotPriceScalingFactorError represents an error that occurs while getting the spot price scaling factor.
type GettingSpotPriceScalingFactorError struct {
	BaseDenom  string
	QuoteDenom string
	Err        error
}

// Error implements the error interface.
func (e GettingSpotPriceScalingFactorError) Error() string {
	return fmt.Sprintf("error getting spot price scaling factor for base denom %s and quote denom %s: %v", e.BaseDenom, e.QuoteDenom, e.Err)
}

// ParsingTickValuesError represents an error that occurs while parsing the tick values.
type ParsingTickValuesError struct {
	Field string
	Err   error
}

// Error implements the error interface.
func (e ParsingTickValuesError) Error() string {
	return fmt.Sprintf("error parsing tick values for field %s: %v", e.Field, e.Err)
}

// ParsingUnrealizedCancelsError represents an error that occurs while parsing the unrealized cancels.
type ParsingUnrealizedCancelsError struct {
	Field string
	Err   error
}

// Error implements the error interface.
func (e ParsingUnrealizedCancelsError) Error() string {
	return fmt.Sprintf("error parsing unrealized cancels for field %s: %v", e.Field, e.Err)
}

// ParsingEtasError represents an error that occurs while parsing the ETAs field.
type ParsingEtasError struct {
	Etas string
	Err  error
}

// Error implements the error interface.
func (e ParsingEtasError) Error() string {
	return fmt.Sprintf("error parsing etas %s: %v", e.Etas, e.Err)
}

// CalculatingPercentFilledError represents an error that occurs while calculating the percent filled.
type CalculatingPercentFilledError struct {
	Err error
}

// Error implements the error interface.
func (e CalculatingPercentFilledError) Error() string {
	return fmt.Sprintf("error calculating percent filled: %v", e.Err)
}

// MappingOrderStatusError represents an error that occurs while mapping the order status.
type MappingOrderStatusError struct {
	Err error
}

// Error implements the error interface.
func (e MappingOrderStatusError) Error() string {
	return fmt.Sprintf("error mapping order status: %v", e.Err)
}

// ConvertingTickToPriceError represents an error that occurs while converting a tick to a price.
type ConvertingTickToPriceError struct {
	TickID int64
	Err    error
}

// Error implements the error interface.
func (e ConvertingTickToPriceError) Error() string {
	return fmt.Sprintf("error converting tick ID %d to price: %v", e.TickID, e.Err)
}

// ParsingPlacedAtError represents an error that occurs while parsing the placed_at field.
type ParsingPlacedAtError struct {
	PlacedAt string
	Err      error
}

// Error implements the error interface.
func (e ParsingPlacedAtError) Error() string {
	return fmt.Sprintf("error parsing placed_at %s: %v", e.PlacedAt, e.Err)
}

// PoolNilError represents an error when the pool is nil.
type PoolNilError struct{}

func (e PoolNilError) Error() string {
	return "pool is nil when processing order book"
}

// CosmWasmPoolModelNilError represents an error when the CosmWasmPoolModel is nil.
type CosmWasmPoolModelNilError struct{}

func (e CosmWasmPoolModelNilError) Error() string {
	return "cw pool model is nil when processing order book"
}

// NotAnOrderbookPoolError represents an error when the pool is not an orderbook pool.
type NotAnOrderbookPoolError struct {
	PoolID uint64
}

func (e NotAnOrderbookPoolError) Error() string {
	return fmt.Sprintf("pool is not an orderbook pool %d", e.PoolID)
}

// FailedToCastPoolModelError represents an error when the pool model cannot be cast to a CosmWasmPool.
type FailedToCastPoolModelError struct{}

func (e FailedToCastPoolModelError) Error() string {
	return "failed to cast pool model to CosmWasmPool"
}

// FetchTicksError represents an error when fetching ticks fails.
type FetchTicksError struct {
	ContractAddress string
	Err             error
}

func (e FetchTicksError) Error() string {
	return fmt.Sprintf("failed to fetch ticks for pool %s: %v", e.ContractAddress, e.Err)
}

// FetchUnrealizedCancelsError represents an error when fetching unrealized cancels fails.
type FetchUnrealizedCancelsError struct {
	ContractAddress string
	Err             error
}

func (e FetchUnrealizedCancelsError) Error() string {
	return fmt.Sprintf("failed to fetch unrealized cancels for pool %s: %v", e.ContractAddress, e.Err)
}

// TickIDMismatchError represents an error when there is a mismatch between tick IDs.
type TickIDMismatchError struct {
	ExpectedID int64
	ActualID   int64
}

func (e TickIDMismatchError) Error() string {
	return fmt.Sprintf("tick id mismatch when fetching tick states %d %d", e.ExpectedID, e.ActualID)
}
