package pools

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
	clmath "github.com/osmosis-labs/osmosis/v25/x/concentrated-liquidity/math"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v25/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v25/x/poolmanager/types"
)

var _ sqsdomain.RoutablePool = &routableOrderbookPoolImpl{}

type routableOrderbookPoolImpl struct {
	ChainPool     *cwpoolmodel.CosmWasmPool   "json:\"pool\""
	Balances      sdk.Coins                   "json:\"balances\""
	TokenOutDenom string                      "json:\"token_out_denom\""
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
	directionIn, err := r.GetDirection(tokenIn.Denom, r.TokenOutDenom)
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
	tickIdx, err := r.GetStartTickIndex(directionOut)
	if err != nil {
		return sdk.Coin{}, err
	}

	amountOutTotal := osmomath.ZeroBigDec()
	amountInRemaining := osmomath.BigDecFromSDKInt(tokenIn.Amount)

	// ASSUMPTION: Ticks are ordered
	for amountInRemaining.GT(zeroBigDec) {
		// Order has run out of ticks to iterate
		if tickIdx >= len(r.OrderbookData.Ticks) || tickIdx < 0 {
			return sdk.Coin{}, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn}
		}
		tick := r.OrderbookData.Ticks[tickIdx]

		// Increment or decrement the current tick index depending on out order direction
		tickIdx += iterationStep

		// Calculate the price for the current tick
		tickPrice, err := clmath.TickToPrice(tick.TickId)
		if err != nil {
			return sdk.Coin{}, err
		}

		// Output amount that should be filled given the current tick price
		outputAmount := convertValue(amountInRemaining, tickPrice, directionOut)

		// Tick liquidity for output side
		outputTickLiquidity, err := getTickLiquidity(&tick.TickLiquidity, directionOut)
		if err != nil {
			return sdk.Coin{}, err
		}

		// How much of the order this tick can fill
		outputFilled := getFillableAmount(outputTickLiquidity, outputAmount)

		// How much of the input denom has been filled
		inputFilled := convertValue(outputFilled, tickPrice, directionIn)

		// Add the filled amount to the order total
		amountOutTotal.AddMut(outputFilled)

		// Subtract the filled amount from the remaining amount of tokens in
		amountInRemaining.SubMut(inputFilled)
	}

	// Return total amount out
	return sdk.Coin{Denom: r.TokenOutDenom, Amount: amountOutTotal.Dec().TruncateInt()}, nil
}

// GetTokenOutDenom implements RoutablePool.
func (r *routableOrderbookPoolImpl) GetTokenOutDenom() string {
	return r.TokenOutDenom
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

// SetTokenOutDenom implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) SetTokenOutDenom(tokenOutDenom string) {
	r.TokenOutDenom = tokenOutDenom
}

// CalcSpotPrice implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) CalcSpotPrice(ctx context.Context, baseDenom string, quoteDenom string) (osmomath.BigDec, error) {
	// Get the expected order direction
	direction, err := r.GetDirection(quoteDenom, baseDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	tickIdx, err := r.GetStartTickIndex(direction.Opposite())
	if err != nil {
		return osmomath.BigDec{}, err
	}

	tick := r.OrderbookData.Ticks[tickIdx]

	// Calculate the price for the current tick
	return clmath.TickToPrice(tick.TickId)
}

// IsGeneralizedCosmWasmPool implements domain.RoutablePool.
func (*routableOrderbookPoolImpl) IsGeneralizedCosmWasmPool() bool {
	return false
}

// GetCodeID implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) GetCodeID() uint64 {
	return r.ChainPool.CodeId
}

// Determines order direction for the current orderbook given token in and out denoms
// Returns:
// - cosmwasmpool.BID (1) if the order is a bid (buying token out)
// - cosmwasmpool.ASK (-1) if the order is an ask (selling token out)
// - 0 if the order is not valid
func (r *routableOrderbookPoolImpl) GetDirection(tokenInDenom, tokenOutDenom string) (domain.OrderbookDirection, error) {
	if tokenInDenom == r.OrderbookData.BaseDenom && tokenOutDenom == r.OrderbookData.QuoteDenom {
		return domain.ASK, nil
	} else if tokenInDenom == r.OrderbookData.QuoteDenom && tokenOutDenom == r.OrderbookData.BaseDenom {
		return domain.BID, nil
	} else {
		return 0, domain.OrderbookPoolMismatchError{PoolId: r.GetId(), TokenInDenom: tokenInDenom, TokenOutDenom: tokenOutDenom}
	}
}

// Get the index for the tick state array for the starting index given direction
func (r *routableOrderbookPoolImpl) GetStartTickIndex(direction domain.OrderbookDirection) (int, error) {
	switch direction {
	case domain.ASK:
		return getTickIndexById(r.OrderbookData, r.OrderbookData.NextAskTick), nil
	case domain.BID:
		return getTickIndexById(r.OrderbookData, r.OrderbookData.NextBidTick), nil
	default:
		return -1, domain.OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

// Converts an amount of token in to the value of token out given a price and direction
func convertValue(amount osmomath.BigDec, price osmomath.BigDec, direction domain.OrderbookDirection) osmomath.BigDec {
	switch direction {
	case domain.ASK:
		return amount.Mul(price)
	case domain.BID:
		return amount.Quo(price)
	default:
		return osmomath.ZeroBigDec()
	}
}

// Returns the related liquidity for a given direction on the current tick
func getTickLiquidity(s *cosmwasmpool.OrderbookTickLiquidity, direction domain.OrderbookDirection) (osmomath.BigDec, error) {
	switch direction {
	case domain.ASK:
		return s.AskLiquidity, nil
	case domain.BID:
		return s.BidLiquidity, nil
	default:
		return osmomath.BigDec{}, domain.OrderbookPoolInvalidDirectionError{Direction: direction}
	}
}

// Returns tick state index for the given ID
func getTickIndexById(d *cosmwasmpool.OrderbookData, tickId int64) int {
	for i, tick := range d.Ticks {
		if tick.TickId == tickId {
			return i
		}
	}
	return -1
}

// Determines how much of a given amount can be filled by the current tick state (independent for each direction)
func getFillableAmount(tickLiquidity osmomath.BigDec, input osmomath.BigDec) osmomath.BigDec {
	if input.LT(tickLiquidity) {
		return input
	}
	return tickLiquidity
}
