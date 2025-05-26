package pools

import (
	"context"
	"fmt"
	"math/big"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	lru "github.com/hashicorp/golang-lru/v2"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/math"

	"github.com/osmosis-labs/osmosis/osmomath"
	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
	concentratedmodel "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/model"
	"github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/swapstrategy"
	cltypes "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/types"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"

	"github.com/ALTree/bigfloat"
)

var (
	_           domain.RoutablePool = &routableConcentratedPoolImpl{}
	smallestDec                     = osmomath.BigDecFromDec(osmomath.SmallestDec())
)

type routableConcentratedPoolImpl struct {
	ChainPool     *concentratedmodel.Pool `json:"cl_pool"`
	TickModel     *ingesttypes.TickModel  `json:"tick_model"`
	TokenInDenom  string                  `json:"token_in_denom,omitempty"`
	TokenOutDenom string                  `json:"token_out_denom,omitempty"`
	TakerFee      osmomath.Dec            `json:"taker_fee"`
}

var (
	sdkOneDec      = mustBigFloatFromDec(osmomath.OneDec())
	sdkTenDec      = mustBigFloatFromDec(osmomath.NewDec(10))
	powersOfTen    []big.Float
	negPowersOfTen []big.Float

	osmomathBigOneDec = osmomath.NewBigDec(1)
	osmomathBigTenDec = osmomath.NewBigDec(10)
	bigPowersOfTen    []big.Float
	bigNegPowersOfTen []big.Float

	// 9 * 10^(-types.ExponentAtPriceOne), where types.ExponentAtPriceOne is non-positive and is s.t.
	// this answer fits well within an int64.
	geometricExponentIncrementDistanceInTicks = 9 * osmomath.NewDec(10).PowerMut(uint64(-cltypes.ExponentAtPriceOne)).TruncateInt64()
)

var MaxSpotPrice = mustBigFloatFromDec(cltypes.MaxSpotPrice)

func mustBigFloatFromBigDec(dec osmomath.BigDec) big.Float {
	f, ok := new(big.Float).SetString(dec.String())
	if !ok {
		panic(fmt.Sprintf("failed to parse %s", dec.String()))
	}
	return *f
}

func mustBigFloatFromDec(dec osmomath.Dec) big.Float {
	f, ok := new(big.Float).SetString(dec.String())
	if !ok {
		panic(fmt.Sprintf("failed to parse %s", dec.String()))
	}
	return *f
}

// Set precision multipliers
func init() {
	// Initialize negPowersOfTen using big.Float
	negPowersOfTen = make([]big.Float, osmomath.DecPrecision+1)
	one := big.NewFloat(1)
	ten := big.NewFloat(10)

	for i := 0; i <= osmomath.DecPrecision; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := big.NewFloat(float64(i))
		power := bigfloat.Pow(ten, exponent)

		// Calculate 1 / 10^i
		negPowersOfTen[i] = *new(big.Float).Quo(one, power)
	}

	// 10^77 < osmomath.MaxInt < 10^78
	powersOfTen = make([]big.Float, 77)
	for i := 0; i <= 76; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := big.NewFloat(float64(i))
		powersOfTen[i] = *bigfloat.Pow(ten, exponent)
	}

	// Initialize bigNegPowersOfTen using big.Float
	bigNegPowersOfTen = make([]big.Float, osmomath.BigDecPrecision+1)
	for i := 0; i <= osmomath.BigDecPrecision; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := big.NewFloat(float64(i))
		power := bigfloat.Pow(ten, exponent)

		// Calculate 1 / 10^i
		bigNegPowersOfTen[i] = *new(big.Float).Quo(one, power)
	}

	// 10^308 < osmomath.MaxInt < 10^309
	bigPowersOfTen = make([]big.Float, 309)
	for i := 0; i <= 308; i++ {
		// Calculate 10^i using bigfloat.Pow
		exponent := big.NewFloat(float64(i))
		bigPowersOfTen[i] = *bigfloat.Pow(ten, exponent)
	}
}

// Builds metadata for every additive tickspacing exponent, namely:
// * what is the first price this tick spacing applies to
// * what is the first tick this applies for
// * (saves on pre-compute) what is the additive increment per tick.
//
// This would be stored in a map, keyed by:
// 0 => (1.00, 10^(types.ExponentAtPriceOne), 0)
// 1 => (10, 10^(1 + types.ExponentAtPriceOne), 9 * types.ExponentAtPriceOne)
// 2 => (100, 10^(2 + types.ExponentAtPriceOne), 9 * (types.ExponentAtPriceOne + 1))
// -1 => (0.1, 10^(types.ExponentAtPriceOne - 1), 9 * (types.ExponentAtPriceOne - 1))
type tickExpIndexData struct {
	// if price < initialPrice, we are not in this exponent range.
	initialPrice big.Float
	// if price >= maxPrice, we are not in this exponent range.
	maxPrice big.Float
	// TODO: Change to normal Dec, if min spot price increases.
	// additive increment per tick here.
	additiveIncrementPerTick big.Float
	// the tick that corresponds to initial price
	initialTick int64
}

var tickExpCache map[int64]*tickExpIndexData = make(map[int64]*tickExpIndexData)

func buildTickExpCache() {
	// build positive indices first
	maxPrice := mustBigFloatFromBigDec(osmomathBigOneDec)
	curExpIndex := int64(0)
	for maxPrice.Cmp(&MaxSpotPrice) < 0 { // LT
		tickExpCache[curExpIndex] = &tickExpIndexData{
			// price range 10^curExpIndex to 10^(curExpIndex + 1). (10, 100)
			initialPrice:             mustBigFloatFromBigDec(osmomathBigTenDec.PowerInteger(uint64(curExpIndex))),
			maxPrice:                 mustBigFloatFromBigDec(osmomathBigTenDec.PowerInteger(uint64(curExpIndex + 1))),
			additiveIncrementPerTick: powTenBigDec(cltypes.ExponentAtPriceOne + curExpIndex),
			initialTick:              geometricExponentIncrementDistanceInTicks * curExpIndex,
		}
		maxPrice = tickExpCache[curExpIndex].maxPrice
		curExpIndex += 1
	}

	minPrice := mustBigFloatFromBigDec(osmomathBigOneDec)
	curExpIndex = -1
	minSpotPrice := mustBigFloatFromBigDec(osmomath.NewBigDecWithPrec(1, 30))
	for minPrice.Cmp(&minSpotPrice) > 0 { // GT
		tickExpCache[curExpIndex] = &tickExpIndexData{
			// price range 10^curExpIndex to 10^(curExpIndex + 1). (0.001, 0.01)
			initialPrice:             powTenBigDec(curExpIndex),
			maxPrice:                 powTenBigDec(curExpIndex + 1),
			additiveIncrementPerTick: powTenBigDec(cltypes.ExponentAtPriceOne + curExpIndex),
			initialTick:              geometricExponentIncrementDistanceInTicks * curExpIndex,
		}
		minPrice = tickExpCache[curExpIndex].initialPrice
		curExpIndex -= 1
	}
}

var oneBigFloat = big.NewFloat(1)

var (
	MinSpotPriceV2     = mustBigFloatFromBigDec(cltypes.MinSpotPriceV2)
	MaxSpotPriceBigDec = mustBigFloatFromBigDec(cltypes.MaxSpotPriceBigDec)
)

// TickToPrice returns the price given a tickIndex
// If tickIndex is zero, the function returns osmomath.OneDec().
func TickToPrice(tickIndex int64) (big.Float, error) {
	if tickIndex == 0 {
		return *oneBigFloat, nil
	}

	// N.B. We special case MinInitializedTickV2 and MinCurrentTickV2 since MinInitializedTickV2
	// is the first one that requires taking 10 to the exponent of (-31 + -6) = -37
	// Given BigDec's precision of 36, that cannot be supported.
	// The fact that MinInitializedTickV2 and MinCurrentTickV2 translate to the same
	// price is acceptable since MinCurrentTickV2 cannot be initialized.
	if tickIndex == cltypes.MinInitializedTickV2 || tickIndex == cltypes.MinCurrentTickV2 {
		return MinSpotPriceV2, nil
	}

	numAdditiveTicks, geometricExponentDelta, err := clmath.TickToAdditiveGeometricIndices(tickIndex)
	if err != nil {
		return big.Float{}, err
	}

	// price = 10^geometricExponentDelta + numAdditiveTicks * 10^exponentAtCurrentTick
	// exponent at current tick = types.ExponentAtPriceOne + geometricExponentDelta + conditional
	// where conditional = -1 if tickIndex < 0, 0 otherwise
	// so we compute the price as (10**(geometricExponentDelta - exponentAtCurrentTick) + numAdditiveTicks) * 10**exponentAtCurrentTick
	// notice that geometricExponentDelta - exponentAtCurrentTick is either 6 or 7
	// so we compute this as unscaledPrice = (10**(geometricExponentDelta - exponentAtCurrentTick) + numAdditiveTicks)

	// Calculate the exponentAtCurrentTick from the starting exponentAtPriceOne and the geometricExponentDelta
	exponentAtCurrentTick := cltypes.ExponentAtPriceOne + geometricExponentDelta
	var unscaledPrice int64 = 1_000_000
	if tickIndex < 0 {
		// We must decrement the exponentAtCurrentTick when entering the negative tick range in order to constantly step up in precision when going further down in ticks
		// Otherwise, from tick 0 to tick -(geometricExponentIncrementDistanceInTicks), we would use the same exponent as the exponentAtPriceOne
		exponentAtCurrentTick = exponentAtCurrentTick - 1
		unscaledPrice *= 10
	}
	unscaledPrice += numAdditiveTicks
	pw := powTenBigDec(exponentAtCurrentTick)
	price := new(big.Float).Mul(&pw, big.NewFloat(float64(unscaledPrice)))

	// defense in depth, this logic would not be reached due to use having checked if given tick is in between
	// min tick and max tick.
	if price.Cmp(&MaxSpotPriceBigDec) > 0 || price.Cmp(&MinSpotPriceV2) < 0 {
		return big.Float{}, cltypes.PriceBoundError{}
		// return big.Float{}, cltypes.PriceBoundError{ProvidedPrice: price, MinSpotPrice: cltypes.MinSpotPriceV2, MaxSpotPrice: cltypes.MaxSpotPrice}
	}
	return *price, nil
}

func powTenBigDec(exponent int64) big.Float {
	if exponent >= 0 {
		return bigPowersOfTen[exponent]
	}
	return bigNegPowersOfTen[-exponent]
}

// TickToSqrtPrice returns the sqrtPrice given a tickIndex
// If tickIndex is zero, the function returns osmomath.OneDec().
// It is the combination of calling TickToPrice followed by Sqrt.
func TickToSqrtPrice_(tickIndex int64) (big.Float, error) {
	priceBigFloat, err := TickToPrice(tickIndex)
	if err != nil {
		return big.Float{}, err
	}

	return *new(big.Float).Sqrt(&priceBigFloat), nil
}

func TickToSqrtPrice(tickIndex int64) (osmomath.BigDec, error) {
	priceBigFloat, err := TickToPrice(tickIndex)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	sqrtPriceFloat := *new(big.Float).Sqrt(&priceBigFloat)

	sqrtPrice, err := math.BigFloatToBigDec(sqrtPriceFloat)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return sqrtPrice, nil
}

// Size is roughly `keys * (2.5 * Key_size + 2*value_size)`. (Plus whatever excess overhead hashmaps internally have)
// key is 8 bytes, value is ~152 bytes
// so at 100k keys its max RAM of ~30MB
var tickToSqrtPriceCache1, _ = lru.New2Q[int64, big.Float](1000000)

// func getTickToSqrtPrice1(tick int64) (big.Float, error) {
// 	if sqrtPrice, ok := tickToSqrtPriceCache1.Get(tick); ok {
// 		return sqrtPrice, nil
// 	}
//
// 	sqrtPrice, err := TickToSqrtPrice_(tick)
// 	if err != nil {
// 		tickToSqrtPriceCache.Add(tick, sqrtPrice)
// 	}
// 	return sqrtPrice, err
// }

var tickToSqrtPriceCache, _ = lru.New2Q[int64, osmomath.BigDec](1000000)

func getTickToSqrtPrice(tick int64) (osmomath.BigDec, error) {
	if sqrtPrice, ok := tickToSqrtPriceCache.Get(tick); ok {
		return sqrtPrice, nil
	}

	sqrtPrice, err := TickToSqrtPrice(tick)
	if err != nil {
		tickToSqrtPriceCache.Add(tick, sqrtPrice)
	}
	return sqrtPrice, err
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) GetPoolDenoms() []string {
	return r.ChainPool.GetPoolDenoms(sdk.Context{})
}

// GetType implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.Concentrated
}

// GetId implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) GetId() uint64 {
	return r.ChainPool.Id
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) GetSpreadFactor() osmomath.Dec {
	return r.ChainPool.SpreadFactor
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) GetTakerFee() osmomath.Dec {
	return r.TakerFee
}

// CalculateTokenOutByTokenIn implements domain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a concentrated liquidity pool.
// Because ChainPool operates on the chain store we simulate the store by operating on the custom data representation that is ingested from chain.
// Fails if:
// - the underlying chain pool set on the routable pool is not of concentrated type
// - fails to retrieve the tick model for the pool
// - the current tick is not within the specified current bucket range
// - tick model has no liquidity flag set
// - the current sqrt price is zero
// - rans out of ticks during swap (token in is too high for liquidity in the pool)
func (r *routableConcentratedPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	concentratedPool := r.ChainPool
	tickModel := r.TickModel

	if tickModel == nil {
		return sdk.Coin{}, domain.ConcentratedPoolNoTickModelError{
			PoolId: r.ChainPool.Id,
		}
	}

	// Ensure pool has liquidity.
	if tickModel.HasNoLiquidity {
		return sdk.Coin{}, domain.ConcentratedNoLiquidityError{
			PoolId: concentratedPool.Id,
		}
	}

	// Ensure that the current bucket is within the available bucket range.
	currentBucketIndex := tickModel.CurrentTickIndex

	if currentBucketIndex < 0 || currentBucketIndex >= int64(len(tickModel.Ticks)) {
		return sdk.Coin{}, domain.ConcentratedCurrentTickNotWithinBucketError{
			PoolId:             concentratedPool.Id,
			CurrentBucketIndex: currentBucketIndex,
			TotalBuckets:       int64(len(tickModel.Ticks)),
		}
	}

	currentBucket := tickModel.Ticks[currentBucketIndex]

	isCurrentTickWithinBucket := concentratedPool.IsCurrentTickInRange(currentBucket.LowerTick, currentBucket.UpperTick)
	if !isCurrentTickWithinBucket {
		return sdk.Coin{}, domain.ConcentratedCurrentTickAndBucketMismatchError{
			PoolID:      concentratedPool.Id,
			CurrentTick: concentratedPool.CurrentTick,
			LowerTick:   currentBucket.LowerTick,
			UpperTick:   currentBucket.UpperTick,
		}
	}

	// Set the appropriate token out denom.
	isZeroForOne := tokenIn.Denom == concentratedPool.Token0
	tokenOutDenom := concentratedPool.Token0
	if isZeroForOne {
		tokenOutDenom = concentratedPool.Token1
	}

	// Initialize the swap strategy.
	swapStrategy := swapstrategy.New(isZeroForOne, smallestDec, &storetypes.KVStoreKey{}, concentratedPool.SpreadFactor)

	var (
		// Swap state
		currentSqrtPrice = concentratedPool.GetCurrentSqrtPrice()

		amountRemainingIn = tokenIn.Amount.ToLegacyDec()
		amountOutTotal    = osmomath.ZeroDec()
	)

	if currentSqrtPrice.IsZero() {
		return sdk.Coin{}, domain.ConcentratedZeroCurrentSqrtPriceError{
			PoolId: concentratedPool.Id,
		}
	}

	// Compute swap over all buckets.
	for amountRemainingIn.IsPositive() {
		if currentBucketIndex >= int64(len(tickModel.Ticks)) || currentBucketIndex < 0 {
			// This happens when there is not enough liquidity in the pool to complete the swap
			// for a given amount of token in.
			return sdk.Coin{}, domain.ConcentratedNotEnoughLiquidityToCompleteSwapError{
				PoolId:   concentratedPool.Id,
				AmountIn: sdk.NewCoins(tokenIn).String(),
			}
		}

		currentBucket = tickModel.Ticks[currentBucketIndex]

		// Compute the next initialized tick index depending on the swap direction.
		// Zero for one - in the lower tick direction.
		// One for zero - in the upper tick direction.
		var nextInitializedTickIndex int64
		if isZeroForOne {
			nextInitializedTickIndex = currentBucket.LowerTick
			currentBucketIndex--
		} else {
			nextInitializedTickIndex = currentBucket.UpperTick
			currentBucketIndex++
		}

		// Get the sqrt price for the next initialized tick index.
		sqrtPriceTarget, err := getTickToSqrtPrice(nextInitializedTickIndex)
		if err != nil {
			return sdk.Coin{}, err
		}

		// Compute the swap within current bucket
		sqrtPriceNext, amountInConsumed, amountOutComputed, spreadRewardChargeTotal := swapStrategy.ComputeSwapWithinBucketOutGivenIn(currentSqrtPrice, sqrtPriceTarget, currentBucket.LiquidityAmount, amountRemainingIn)

		// Update swap state for next iteration
		amountRemainingIn = amountRemainingIn.SubMut(amountInConsumed).SubMut(spreadRewardChargeTotal)
		amountOutTotal = amountOutTotal.AddMut(amountOutComputed)

		// Update current sqrt price
		currentSqrtPrice = sqrtPriceNext
	}

	// Return the total amount out.

	return sdk.Coin{Denom: tokenOutDenom, Amount: amountOutTotal.TruncateInt()}, nil
}

// CalculateTokenInByTokenOut implements domain.RoutablePool.
// It calculates the amount of token in given the amount of token out for a concentrated liquidity pool.
// Because ChainPool operates on the chain store we simulate the store by operating on the custom data representation that is ingested from chain.
// Fails if:
// - the underlying chain pool set on the routable pool is not of concentrated type
// - fails to retrieve the tick model for the pool
// - the current tick is not within the specified current bucket range
// - tick model has no liquidity flag set
// - the current sqrt price is zero
// - rans out of ticks during swap (token out is too high for liquidity in the pool)
func (r *routableConcentratedPoolImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
	concentratedPool := r.ChainPool
	tickModel := r.TickModel

	if tickModel == nil {
		return sdk.Coin{}, domain.ConcentratedPoolNoTickModelError{
			PoolId: r.ChainPool.Id,
		}
	}

	// Ensure pool has liquidity.
	if tickModel.HasNoLiquidity {
		return sdk.Coin{}, domain.ConcentratedNoLiquidityError{
			PoolId: concentratedPool.Id,
		}
	}

	// Ensure that the current bucket is within the available bucket range.
	currentBucketIndex := tickModel.CurrentTickIndex

	if currentBucketIndex < 0 || currentBucketIndex >= int64(len(tickModel.Ticks)) {
		return sdk.Coin{}, domain.ConcentratedCurrentTickNotWithinBucketError{
			PoolId:             concentratedPool.Id,
			CurrentBucketIndex: currentBucketIndex,
			TotalBuckets:       int64(len(tickModel.Ticks)),
		}
	}

	currentBucket := tickModel.Ticks[currentBucketIndex]

	isCurrentTickWithinBucket := concentratedPool.IsCurrentTickInRange(currentBucket.LowerTick, currentBucket.UpperTick)
	if !isCurrentTickWithinBucket {
		return sdk.Coin{}, domain.ConcentratedCurrentTickAndBucketMismatchError{
			PoolID:      concentratedPool.Id,
			CurrentTick: concentratedPool.CurrentTick,
			LowerTick:   currentBucket.LowerTick,
			UpperTick:   currentBucket.UpperTick,
		}
	}

	// Set the appropriate token out denom.
	isZeroForOne := tokenOut.Denom == concentratedPool.Token1
	tokenInDenom := concentratedPool.Token1
	if isZeroForOne {
		tokenInDenom = concentratedPool.Token0
	}

	// Initialize the swap strategy.
	swapStrategy := swapstrategy.New(isZeroForOne, smallestDec, &storetypes.KVStoreKey{}, concentratedPool.SpreadFactor)

	var (
		// Swap state
		currentSqrtPrice = concentratedPool.GetCurrentSqrtPrice()

		amountRemainingOut = tokenOut.Amount.ToLegacyDec()
		amountInTotal      = osmomath.ZeroDec()
	)

	if currentSqrtPrice.IsZero() {
		return sdk.Coin{}, domain.ConcentratedZeroCurrentSqrtPriceError{
			PoolId: concentratedPool.Id,
		}
	}

	// Compute swap over all buckets.
	for amountRemainingOut.IsPositive() {
		if currentBucketIndex >= int64(len(tickModel.Ticks)) || currentBucketIndex < 0 {
			// This happens when there is not enough liquidity in the pool to complete the swap
			// for a given amount of token in.
			return sdk.Coin{}, domain.ConcentratedNotEnoughLiquidityToCompleteSwapError{
				PoolId:    concentratedPool.Id,
				AmountOut: sdk.NewCoins(tokenOut).String(),
			}
		}

		currentBucket = tickModel.Ticks[currentBucketIndex]

		// Compute the next initialized tick index depending on the swap direction.
		// Zero for one - in the lower tick direction.
		// One for zero - in the upper tick direction.
		var nextInitializedTickIndex int64
		if isZeroForOne {
			nextInitializedTickIndex = currentBucket.LowerTick
			currentBucketIndex--
		} else {
			nextInitializedTickIndex = currentBucket.UpperTick
			currentBucketIndex++
		}

		// Get the sqrt price for the next initialized tick index.
		sqrtPriceTarget, err := getTickToSqrtPrice(nextInitializedTickIndex)
		if err != nil {
			return sdk.Coin{}, err
		}

		// Compute the swap within current bucket
		sqrtPriceNext, amountOutConsumed, amountInComputed, spreadRewardChargeTotal := swapStrategy.ComputeSwapWithinBucketInGivenOut(currentSqrtPrice, sqrtPriceTarget, currentBucket.LiquidityAmount, amountRemainingOut)

		// Update swap state for next iteration
		amountRemainingOut = amountRemainingOut.SubMut(amountOutConsumed).SubMut(spreadRewardChargeTotal)
		amountInTotal = amountInTotal.AddMut(amountInComputed)

		// Update current sqrt price
		currentSqrtPrice = sqrtPriceNext
	}

	// Return the total amount in.
	return sdk.Coin{Denom: tokenInDenom, Amount: amountInTotal.TruncateInt()}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableConcentratedPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *routableConcentratedPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// String implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) String() string {
	concentratedPool := r.ChainPool
	return fmt.Sprintf("pool (%d), pool type (%d), pool denoms (%v), token out (%s)", concentratedPool.Id, poolmanagertypes.Concentrated, concentratedPool.GetPoolDenoms(sdk.Context{}), r.TokenOutDenom)
}

// ChargeTakerFeeExactIn implements domain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *routableConcentratedPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// ChargeTakerFeeExactOut implements domain.RoutablePool.
// Charges the taker fee for the given token out and returns the token out after the fee has been charged.
func (r *routableConcentratedPoolImpl) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (tokenOutAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactOut(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	spotPrice, err := r.ChainPool.SpotPrice(sdk.Context{}.WithContext(ctx), quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}
	return spotPrice, nil
}

// GetSQSType implements domain.RoutablePool.
func (*routableConcentratedPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.Concentrated
}

// GetCodeID implements domain.RoutablePool.
func (r *routableConcentratedPoolImpl) GetCodeID() uint64 {
	return notCosmWasmPoolCodeID
}
