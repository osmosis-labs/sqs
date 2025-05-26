package swapstrategy

import (
	"fmt"
	"math"
)

// Helper functions for float64 calculations that were previously done with osmomath

// CalcAmount0Delta calculates the amount of token 0 delta
func CalcAmount0Delta(liquidity, sqrtPriceA, sqrtPriceB float64, roundUp bool) float64 {
	if sqrtPriceA > sqrtPriceB {
		sqrtPriceA, sqrtPriceB = sqrtPriceB, sqrtPriceA
	}
	diff := sqrtPriceB - sqrtPriceA

	if sqrtPriceA == 0 || sqrtPriceB == 0 {
		panic(fmt.Sprintf("CalcAmount0Delta: sqrt price cannot be zero: sqrtPriceA %f sqrtPriceB %f", sqrtPriceA, sqrtPriceB))
	}

	result := (liquidity * diff) / (sqrtPriceB * sqrtPriceA)

	if roundUp {
		return math.Ceil(result*1e18) / 1e18
	}
	return math.Floor(result*1e18) / 1e18
}

// CalcAmount1Delta calculates the amount of token 1 delta
func CalcAmount1Delta(liquidity, sqrtPriceA, sqrtPriceB float64, roundUp bool) float64 {
	diff := math.Abs(sqrtPriceB - sqrtPriceA)
	result := liquidity * diff

	if roundUp {
		return math.Ceil(result*1e18) / 1e18
	}
	return math.Floor(result*1e18) / 1e18
}

// GetNextSqrtPriceFromAmount0InRoundingUp calculates next sqrt price from amount 0 in
func GetNextSqrtPriceFromAmount0InRoundingUp(sqrtPriceCurrent, liquidity, amountZeroRemainingIn float64) float64 {
	if amountZeroRemainingIn == 0 {
		return sqrtPriceCurrent
	}

	product := amountZeroRemainingIn * sqrtPriceCurrent
	denominator := product + liquidity

	result := (liquidity * sqrtPriceCurrent) / denominator
	return math.Ceil(result*1e18) / 1e18
}

// GetNextSqrtPriceFromAmount0OutRoundingUp calculates next sqrt price from amount 0 out
func GetNextSqrtPriceFromAmount0OutRoundingUp(sqrtPriceCurrent, liquidity, amountZeroRemainingOut float64) float64 {
	if amountZeroRemainingOut == 0 {
		return sqrtPriceCurrent
	}

	product := sqrtPriceCurrent * amountZeroRemainingOut
	denominator := liquidity - product

	if denominator <= 0 {
		panic("GetNextSqrtPriceFromAmount0OutRoundingUp: denominator must be positive")
	}

	result := (liquidity * sqrtPriceCurrent) / denominator
	return math.Ceil(result*1e18) / 1e18
}

// GetNextSqrtPriceFromAmount1InRoundingDown calculates next sqrt price from amount 1 in
func GetNextSqrtPriceFromAmount1InRoundingDown(sqrtPriceCurrent, liquidity, amountOneRemainingIn float64) float64 {
	result := amountOneRemainingIn/liquidity + sqrtPriceCurrent
	return math.Floor(result*1e18) / 1e18
}

// GetNextSqrtPriceFromAmount1OutRoundingDown calculates next sqrt price from amount 1 out
func GetNextSqrtPriceFromAmount1OutRoundingDown(sqrtPriceCurrent, liquidity, amountOneRemainingOut float64) float64 {
	result := sqrtPriceCurrent - (amountOneRemainingOut / liquidity)
	return math.Floor(result*1e18) / 1e18
}

// floatEqual checks if two floats are approximately equal
func floatEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
