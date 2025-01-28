package domain

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	// ErrInternalServerError will throw if any the Internal Server Error happen
	ErrInternalServerError = errors.New("internal Server Error")
	// ErrNotFound will throw if the requested item is not exists
	ErrNotFound = errors.New("your requested Item is not found")
	// ErrConflict will throw if the current action already exists
	ErrConflict = errors.New("your Item already exist")
	// ErrBadParamInput will throw if the given request-body or params is not valid
	ErrBadParamInput = errors.New("given Param is not valid")
)

var (
	ErrBaseDenomNotValid       = errors.New("base denom is empty")
	ErrQuoteDenomNotValid      = errors.New("quote denom is empty")
	ErrPoolIDNotValid          = errors.New("pool ID is zero")
	ErrContractAddressNotValid = errors.New("contract address is empty")
)

// GetStatusCode returbs status code given error
func GetStatusCode(err error) int {
	if err == nil {
		return http.StatusOK
	}

	switch err {
	case ErrInternalServerError:
		return http.StatusInternalServerError
	case ErrNotFound:
		return http.StatusNotFound
	case ErrConflict:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

// ResponseError represent the response error struct
type ResponseError struct {
	Message string `json:"message"`
}

// InvalidPoolTypeError is an error type for invalid pool type.
type InvalidPoolTypeError struct {
	PoolType int32
}

func (e InvalidPoolTypeError) Error() string {
	return "invalid pool type: " + string(e.PoolType)
}

// UnsupportedCosmWasmPoolError is an error type for unsupported CosmWasm pool.
type UnsupportedCosmWasmPoolError struct {
	PoolId uint64
}

func (e UnsupportedCosmWasmPoolError) Error() string {
	return fmt.Sprintf("Pool %d is a CosmWasm pool but is not supported", e.PoolId)
}

type PoolNotFoundError struct {
	PoolID uint64
}

func (e PoolNotFoundError) Error() string {
	return fmt.Sprintf("pool with ID (%d) is not found", e.PoolID)
}

type ConcentratedPoolNoTickModelError struct {
	PoolId uint64
}

func (e ConcentratedPoolNoTickModelError) Error() string {
	return fmt.Sprintf("concentrated pool (%d) has no tick model", e.PoolId)
}

type TakerFeeNotFoundForDenomPairError struct {
	Denom0 string
	Denom1 string
}

func (e TakerFeeNotFoundForDenomPairError) Error() string {
	return fmt.Sprintf("taker fee not found for denom pair (%s, %s)", e.Denom0, e.Denom1)
}

type FailedToCastPoolModelError struct {
	ExpectedModel string
	ActualModel   string
}

func (e FailedToCastPoolModelError) Error() string {
	return fmt.Sprintf("failed to cast pool model (%s) to the desired type (%s)", e.ActualModel, e.ExpectedModel)
}

type ConcentratedNoLiquidityError struct {
	PoolId uint64
}

func (e ConcentratedNoLiquidityError) Error() string {
	return fmt.Sprintf("pool (%d) has no liquidity", e.PoolId)
}

type ConcentratedCurrentTickNotWithinBucketError struct {
	PoolId             uint64
	CurrentBucketIndex int64
	TotalBuckets       int64
}

func (e ConcentratedCurrentTickNotWithinBucketError) Error() string {
	return fmt.Sprintf("current bucket index (%d) is out of range (%d) for pool (%d)", e.CurrentBucketIndex, e.TotalBuckets, e.PoolId)
}

type ConcentratedCurrentTickAndBucketMismatchError struct {
	PoolID      uint64
	CurrentTick int64
	LowerTick   int64
	UpperTick   int64
}

func (e ConcentratedCurrentTickAndBucketMismatchError) Error() string {
	return fmt.Sprintf("current tick (%d) is not within bucket (%d, %d)", e.CurrentTick, e.LowerTick, e.UpperTick)
}

type ConcentratedZeroCurrentSqrtPriceError struct {
	PoolId uint64
}

func (e ConcentratedZeroCurrentSqrtPriceError) Error() string {
	return fmt.Sprintf("pool (%d) has zero current sqrt price", e.PoolId)
}

type ConcentratedNotEnoughLiquidityToCompleteSwapError struct {
	PoolId    uint64
	AmountIn  string
	AmountOut string
}

func (e ConcentratedNotEnoughLiquidityToCompleteSwapError) Error() string {
	if e.AmountIn != "" {
		return fmt.Sprintf("not enough liquidity to complete swap in pool (%d) with amount in (%s)", e.PoolId, e.AmountIn)
	}

	return fmt.Sprintf("not enough liquidity to complete swap in pool (%d) with amount out (%s)", e.PoolId, e.AmountOut)
}

type ConcentratedTickModelNotSetError struct {
	PoolId uint64
}

func (e ConcentratedTickModelNotSetError) Error() string {
	return fmt.Sprintf("tick model is not set on pool (%d)", e.PoolId)
}

// CosmWasmPoolType represents the type of a CosmWasm pool.
type CosmWasmPoolType int

const (
	CosmWasmPoolTransmuter CosmWasmPoolType = iota
	CosmWasmPoolAlloyTransmuter
	CosmWasmPoolOrderbook
	CosmWasmPoolGeneralized
)

// String returns the string representation of the CwPoolType.
func (c CosmWasmPoolType) String() string {
	switch c {
	case CosmWasmPoolTransmuter:
		return "Transmuter"
	case CosmWasmPoolAlloyTransmuter:
		return "Alloy Transmuter"
	case CosmWasmPoolOrderbook:
		return "Orderbook"
	case CosmWasmPoolGeneralized:
		return "Generalized"
	default:
		return "Unknown"
	}
}

type CosmWasmPoolDataMissingError struct {
	PoolId           uint64
	CosmWasmPoolType CosmWasmPoolType
}

func (e CosmWasmPoolDataMissingError) Error() string {
	return fmt.Sprintf("%s data is missing for pool (%d)", e.CosmWasmPoolType, e.PoolId)
}

type MissingNormalizationFactorError struct {
	PoolId uint64
	Denom  string
}

func (e MissingNormalizationFactorError) Error() string {
	return fmt.Sprintf("Missing normalization factor for denom (%s) in pool (%d)", e.Denom, e.PoolId)
}

type ZeroNormalizationFactorError struct {
	PoolId uint64
	Denom  string
}

func (e ZeroNormalizationFactorError) Error() string {
	return fmt.Sprintf("Normalization factor is zero for denom (%s) in pool (%d)", e.Denom, e.PoolId)
}

type TransmuterInsufficientBalanceError struct {
	Denom         string
	BalanceAmount string
	Amount        string
}

func (e TransmuterInsufficientBalanceError) Error() string {
	return fmt.Sprintf("insufficient balance of token (%s), balance (%s), amount (%s)", e.Denom, e.BalanceAmount, e.Amount)
}

type StaleHeightError struct {
	StoredHeight            uint64
	TimeSinceLastUpdate     int
	MaxAllowedTimeDeltaSecs int
}

func (e StaleHeightError) Error() string {
	return fmt.Sprintf("stored height (%d) is stale, time since last update (%d), max allowed seconds (%d)", e.StoredHeight, e.TimeSinceLastUpdate, e.MaxAllowedTimeDeltaSecs)
}

type PoolDenomMetaDataNotPresentError struct {
	ChainDenom string
}

func (e PoolDenomMetaDataNotPresentError) Error() string {
	return fmt.Sprintf("pool denom metadata for denom (%s) is not found", e.ChainDenom)
}

type SameDenomError struct {
	DenomA string
	DenomB string
}

func (e SameDenomError) Error() string {
	return fmt.Sprintf("two input denoms are equal (%s), must not be the same", e.DenomA)
}

type SpotPriceQuoteCalculatorOutAmountZeroError struct {
	QuoteCoinStr string
	BaseDenom    string
}

func (e SpotPriceQuoteCalculatorOutAmountZeroError) Error() string {
	return fmt.Sprintf("out amount is zero when attempting to compute spot price via quote, quote coin (%s), base denom (%s)", e.QuoteCoinStr, e.BaseDenom)
}

type SpotPriceQuoteCalculatorTruncatedError struct {
	QuoteCoinStr string
	BaseDenom    string
}

func (e SpotPriceQuoteCalculatorTruncatedError) Error() string {
	return fmt.Sprintf("spot price truncated when using quote method, quote coin (%s), base denom (%s)", e.QuoteCoinStr, e.BaseDenom)
}

type OrderbookNotEnoughLiquidityToCompleteSwapError struct {
	PoolId   uint64
	AmountIn string
}

func (e OrderbookNotEnoughLiquidityToCompleteSwapError) Error() string {
	return fmt.Sprintf("not enough liquidity to complete swap in pool (%d) with amount in (%s)", e.PoolId, e.AmountIn)
}

type OrderbookTickIndexOutOfBoundError struct {
	PoolId       uint64
	TickIndex    int
	MaxTickIndex int
}

func (e OrderbookTickIndexOutOfBoundError) Error() string {
	return fmt.Sprintf("tick index (%d) is out of bound for pool (%d), max tick index is (%d)", e.TickIndex, e.PoolId, e.MaxTickIndex)
}

type DenomPoolLiquidityDataNotFoundError struct {
	Denom string
}

func (e DenomPoolLiquidityDataNotFoundError) Error() string {
	return fmt.Sprintf("denom pool liquidity data not found for denom %s", e.Denom)
}

type PriceNotFoundForPoolLiquidityCapError struct {
	Denom string
}

func (e PriceNotFoundForPoolLiquidityCapError) Error() string {
	return fmt.Sprintf("price not found (zero) for denom %s", e.Denom)
}

type FailCastCanonicalOrderbookEntryError struct {
	BaseQuoteKey string
}

func (e FailCastCanonicalOrderbookEntryError) Error() string {
	return fmt.Sprintf("failed to cast orderbook entry for key (%s)", e.BaseQuoteKey)
}

type FailSplitCanonicalOrderBookKeyError struct {
	BaseQuoteKey string
}

func (e FailSplitCanonicalOrderBookKeyError) Error() string {
	return fmt.Sprintf("failed to split base and quote denom from key %s", e.BaseQuoteKey)
}

type FailCastCanonicalOrderbookKeyError struct {
	BaseQuoteKey string
}

func (e FailCastCanonicalOrderbookKeyError) Error() string {
	return fmt.Sprintf("failed to cast key with value %v, expected string", e.BaseQuoteKey)
}

type StaticRateLimiterInvalidUpperLimitError struct {
	UpperLimit string
	Weight     string
	Denom      string
}

func (e StaticRateLimiterInvalidUpperLimitError) Error() string {
	return fmt.Sprintf("invalid upper limit (%s) for weight (%s) and denom (%s)", e.UpperLimit, e.Weight, e.Denom)
}
