package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

var oneBigDec = osmomath.OneBigDec()

var _ domain.RoutablePool = &routableOrderbookPoolImpl{}

type routableOrderbookPoolImpl struct {
	ChainPool     *cwpoolmodel.CosmWasmPool   "json:\"pool\""
	Balances      sdk.Coins                   "json:\"balances\""
	TokenInDenom  string                      "json:\"token_in_denom,omitempty\""
	TokenOutDenom string                      "json:\"token_out_denom,omitempty\""
	TakerFee      osmomath.Dec                "json:\"taker_fee\""
	SpreadFactor  osmomath.Dec                "json:\"spread_factor\""
	OrderbookData *cosmwasmpool.OrderbookData "json:\"orderbook_data\""
}

// GetId implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetId() uint64 {
	return r.ChainPool.PoolId
}

// GetPoolDenoms implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetPoolDenoms() []string {
	return r.Balances.Denoms()
}

// GetType implements domain.RoutablePool.
func (*routableOrderbookPoolImpl) GetType() poolmanagertypes.PoolType {
	return poolmanagertypes.CosmWasm
}

// GetSpreadFactor implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetSpreadFactor() math.LegacyDec {
	return r.SpreadFactor
}

// CalculateTokenOutByTokenIn implements sqsdomain.RoutablePool.
// It calculates the amount of token out given the amount of token in for a orderbook pool.
// Fails if:
// - the underlying chain pool set on the routable pool is not of cosmwasm type
// - token in and token out denoms are the same
// - the provided denom pair is not supported by the orderbook
// - fails to retrieve the tick model for the pool
// - runs out of ticks during swap (token in is too high for liquidity in the pool)
// - `TickToPrice` calculation fails
func (r *routableOrderbookPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	poolType := r.GetType()

	// Esnure that the pool is a cosmwasm pool
	if poolType != poolmanagertypes.CosmWasm {
		return sdk.Coin{}, domain.InvalidPoolTypeError{PoolType: int32(poolType)}
	}

	// Get the expected order directionIn
	directionIn, err := r.OrderbookData.GetDirection(tokenIn.Denom, r.TokenOutDenom)
	if err != nil {
		return sdk.Coin{}, err
	}
	directionOut := directionIn.Opposite()
	iterationStep, err := directionOut.IterationStep()
	if err != nil {
		return sdk.Coin{}, err
	}

	// Get starting tick index for the "out" side of the orderbook
	// Since the order will get the liquidity out from that side
	tickIdx, err := r.OrderbookData.GetStartTickIndex(directionOut)
	if err != nil {
		return sdk.Coin{}, err
	}

	amountOutTotal := osmomath.ZeroBigDec()
	amountInRemaining := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	var amountInToExhaustLiquidity osmomath.BigDec
	if *directionIn == cosmwasmpool.BID {
		amountInToExhaustLiquidity = r.OrderbookData.BidAmountToExhaustAskLiquidity
	} else {
		amountInToExhaustLiquidity = r.OrderbookData.AskAmountToExhaustBidLiquidity
	}

	// check if amount in > amountInToExhaustLiquidity, if so this swap is not possible due to insufficient liquidity
	if amountInRemaining.GT(amountInToExhaustLiquidity) {
		return sdk.Coin{}, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn.String()}
	}

	// ASSUMPTION: Ticks are ordered
	for amountInRemaining.GT(smallestDec) {
		// Order has run out of ticks to iterate
		if tickIdx >= len(r.OrderbookData.Ticks) || tickIdx < 0 {
			return sdk.Coin{}, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn.String()}
		}

		// According to the check on amountInToExhaustLiquidity above, we should never run out of ticks here
		tick := r.OrderbookData.Ticks[tickIdx]

		// Calculate the price for the current tick
		tickPrice, err := clmath.TickToPrice(tick.TickId)
		if err != nil {
			return sdk.Coin{}, err
		}

		// Amount that should be filled given the current tick price and all the remaining amount of tokens in
		// if the current tick has enough liquidity

		outputAmount := cosmwasmpool.OrderbookValueInOppositeDirection(amountInRemaining, tickPrice, *directionIn, cosmwasmpool.ROUND_DOWN)

		// Cap the output amount to the amount of tokens that can be filled in the current tick
		outputFilled := tick.TickLiquidity.GetFillableAmount(outputAmount, directionOut)

		// Convert the filled amount back to the input amount that should be deducted
		// from the remaining amount of tokens in
		inputFilled := cosmwasmpool.OrderbookValueInOppositeDirection(outputFilled, tickPrice, directionOut, cosmwasmpool.ROUND_UP)

		// Note: left for convinience for debugging
		// fmt.Println("amountInRemaining", amountInRemaining)
		// fmt.Println("tickPrice", tickPrice)
		// fmt.Println("tickIdx", tickIdx)
		// fmt.Println("tickId", tick.TickId)
		// fmt.Println("outputFilled", outputFilled)
		// fmt.Println("inputFilled", inputFilled)
		// fmt.Println("ask liquidity", tick.TickLiquidity.AskLiquidity)
		// fmt.Println("bid liquidity", tick.TickLiquidity.BidLiquidity)

		// Add the filled amount to the order total
		amountOutTotal.AddMut(outputFilled)

		// Subtract the filled amount from the remaining amount of tokens in
		amountInRemaining.SubMut(inputFilled)

		// Increment or decrement the current tick index depending on out order direction
		tickIdx += iterationStep
	}

	// Return total amount out
	return sdk.Coin{Denom: r.TokenOutDenom, Amount: amountOutTotal.Dec().TruncateInt()}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableOrderbookPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
}

// GetTokenInDenom implements RoutablePool.
func (r *routableOrderbookPoolImpl) GetTokenInDenom() string {
	return r.TokenInDenom
}

// String implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) String() string {
	return fmt.Sprintf("pool (%d), pool type (%d) Orderbook, pool denoms (%v), token out (%s)", r.ChainPool.PoolId, poolmanagertypes.CosmWasm, r.GetPoolDenoms(), r.TokenOutDenom)
}

// ChargeTakerFee implements sqsdomain.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *routableOrderbookPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// GetTakerFee implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetTakerFee() math.LegacyDec {
	return r.TakerFee
}

// SetTokenInDenom implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) SetTokenInDenom(tokenInDenom string) {
	r.TokenInDenom = tokenInDenom
}

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// Get the expected order directionIn
	directionIn, err := r.OrderbookData.GetDirection(baseDenom, quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	directionOut := directionIn.Opposite()
	tickIdx, err := r.OrderbookData.GetStartTickIndex(directionOut)

	if err != nil {
		return osmomath.BigDec{}, err
	}

	if tickIdx >= len(r.OrderbookData.Ticks) {
		return osmomath.BigDec{}, domain.OrderbookTickIndexOutOfBoundError{
			PoolId:       r.GetId(),
			TickIndex:    tickIdx,
			MaxTickIndex: len(r.OrderbookData.Ticks) - 1,
		}
	}

	if tickIdx < 0 {
		return osmomath.BigDec{}, cosmwasmpool.OrderbookOrderNotAvailableError{
			PoolId:    r.GetId(),
			Direction: directionOut,
		}
	}

	tick := r.OrderbookData.Ticks[tickIdx]

	// Calculate the price for the current tick
	tickPrice, err := clmath.TickToPrice(tick.TickId)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	return cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, *directionIn, cosmwasmpool.ROUND_DOWN), nil
}

// IsGeneralizedCosmWasmPool implements domain.RoutablePool.
func (*routableOrderbookPoolImpl) IsGeneralizedCosmWasmPool() bool {
	return false
}

// GetCodeID implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}

// GetSQSType implements domain.RoutablePool.
func (*routableOrderbookPoolImpl) GetSQSType() domain.SQSPoolType {
	return domain.Orderbook
}
