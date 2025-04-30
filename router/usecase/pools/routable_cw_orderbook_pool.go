package pools

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	"github.com/osmosis-labs/sqs/domain"

	"github.com/osmosis-labs/osmosis/osmomath"
	clmath "github.com/osmosis-labs/osmosis/v28/x/concentrated-liquidity/math"
	cwpoolmodel "github.com/osmosis-labs/osmosis/v28/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v28/x/poolmanager"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
)

var oneBigDec = osmomath.OneBigDec()

var _ domain.RoutablePool = &routableOrderbookPoolImpl{}

type routableOrderbookPoolImpl struct {
	ChainPool     *cwpoolmodel.CosmWasmPool   `json:"pool"`
	Balances      sdk.Coins                   `json:"balances"`
	TokenInDenom  string                      `json:"token_in_denom,omitempty"`
	TokenOutDenom string                      `json:"token_out_denom,omitempty"`
	TakerFee      osmomath.Dec                `json:"taker_fee"`
	SpreadFactor  osmomath.Dec                `json:"spread_factor"`
	OrderbookData *cosmwasmpool.OrderbookData `json:"orderbook_data"`
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

// CalculateTokenOutByTokenIn implements ingesttypes.RoutablePool.
// It calculates the amount of token out given the amount of token in for a orderbook pool.
// Fails if:
// - the underlying chain pool set on the routable pool is not of cosmwasm type
// - token in and token out denoms are the same
// - the provided denom pair is not supported by the orderbook
// - fails to retrieve the tick model for the pool
// - runs out of ticks during swap (token in is too high for liquidity in the pool)
// - `TickToPrice` calculation fails
func (r *routableOrderbookPoolImpl) CalculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, error) {
	tokenOut, _, err := r.calculateTokenOutByTokenIn(ctx, tokenIn)
	if err != nil {
		return sdk.Coin{}, err
	}
	return tokenOut, nil
}

func (r *routableOrderbookPoolImpl) calculateTokenOutByTokenIn(ctx context.Context, tokenIn sdk.Coin) (sdk.Coin, []osmomath.BigDec, error) {
	poolType := r.GetType()

	if v := ctx.Value(domain.DebugKey); v != nil && r.GetId() == 2140 {
		tokenIn.Denom = tokenIn.Denom
	}

	// Esnure that the pool is a cosmwasm pool
	if poolType != poolmanagertypes.CosmWasm {
		return sdk.Coin{}, nil, domain.InvalidPoolTypeError{PoolType: int32(poolType)}
	}

	// Get the expected order directionIn
	directionIn, err := r.OrderbookData.GetDirection(tokenIn.Denom, r.TokenOutDenom)
	if err != nil {
		return sdk.Coin{}, nil, err
	}
	directionOut := directionIn.Opposite()
	iterationStep, err := directionOut.IterationStep()
	if err != nil {
		return sdk.Coin{}, nil, err
	}

	// Get starting tick index for the "out" side of the orderbook
	// Since the order will get the liquidity out from that side
	tickIdx, err := r.OrderbookData.GetStartTickIndex(directionOut)
	if err != nil {
		return sdk.Coin{}, nil, err
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
		return sdk.Coin{}, nil, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn.String()}
	}

	// Tick prices across the orderbook used to fill the order
	var tickPrices []osmomath.BigDec

	// ASSUMPTION: Ticks are ordered
	for amountInRemaining.GT(smallestDec) {
		// Order has run out of ticks to iterate
		if tickIdx >= len(r.OrderbookData.Ticks) || tickIdx < 0 {
			return sdk.Coin{}, nil, domain.OrderbookNotEnoughLiquidityToCompleteSwapError{PoolId: r.GetId(), AmountIn: tokenIn.String()}
		}

		// According to the check on amountInToExhaustLiquidity above, we should never run out of ticks here
		tick := r.OrderbookData.Ticks[tickIdx]

		// Calculate the price for the current tick
		tickPrice, err := clmath.TickToPrice(tick.TickId)
		if err != nil {
			return sdk.Coin{}, nil, err
		}

		// Store the tick price
		tickPrices = append(tickPrices, tickPrice)

		// Amount that should be filled given the current tick price and all the remaining amount of tokens in
		// if the current tick has enough liquidity

		outputAmount := cosmwasmpool.OrderbookValueInOppositeDirection(amountInRemaining, tickPrice, *directionIn, cosmwasmpool.ROUND_DOWN)

		// Cap the output amount to the amount of tokens that can be filled in the current tick
		outputFilled := tick.TickLiquidity.GetFillableAmount(outputAmount, directionOut) // 23.899477

		// Convert the filled amount back to the input amount that should be deducted
		// from the remaining amount of tokens in
		inputFilled := cosmwasmpool.OrderbookValueInOppositeDirection(outputFilled, tickPrice, directionOut, cosmwasmpool.ROUND_UP) // 238.994770

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
	return sdk.Coin{Denom: r.TokenOutDenom, Amount: amountOutTotal.Dec().TruncateInt()}, tickPrices, nil
}

// CalculateTokenInByTokenOut implements domain.RoutablePool.
func (r *routableOrderbookPoolImpl) CalculateTokenInByTokenOut(ctx context.Context, tokenOut sdk.Coin) (sdk.Coin, error) {
	return sdk.Coin{}, errors.New("not implemented")
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

// ChargeTakerFee implements ingesttypes.RoutablePool.
// Charges the taker fee for the given token in and returns the token in after the fee has been charged.
func (r *routableOrderbookPoolImpl) ChargeTakerFeeExactIn(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	tokenInAfterTakerFee, _ := poolmanager.CalcTakerFeeExactIn(tokenIn, r.GetTakerFee())
	return tokenInAfterTakerFee
}

// ChargeTakerFee implements sqsdomain.RoutablePool.
// Charges the taker fee for the given token out and returns the token out after the fee has been charged.
func (r *routableOrderbookPoolImpl) ChargeTakerFeeExactOut(tokenIn sdk.Coin) (tokenInAfterFee sdk.Coin) {
	return sdk.Coin{}
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

	// We're in the same direction, spot price is the same as the tick price
	if r.OrderbookData.BaseDenom == baseDenom && r.OrderbookData.QuoteDenom == quoteDenom {
		return cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, *directionIn, cosmwasmpool.ROUND_DOWN), nil
	}

	// In the opposite direction, we need to invert the tick price.
	// For example, orderbook with base denom TRX and quote denom USDC, and tick price is 10.
	// When quote token in is USDC and token out is TRX, the spot price is 0.1, because 1 TRX is 10 USDC
	// but we want spot price in terms of USDC, not TRX, thus we invert 0.1 to 10.
	return cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, directionOut, cosmwasmpool.ROUND_DOWN), nil
}

func (r *routableOrderbookPoolImpl) CalcSpotPriceInGivenOut(ctx context.Context, tokenIn sdk.Coin, tokenDenomOut string) (osmomath.BigDec, error) {
	// return r.CalcSpotPrice(ctx, tokenIn.Denom, tokenDenomOut)
	_, tickPrices, err := r.calculateTokenOutByTokenIn(ctx, tokenIn)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	baseDenom, quoteDenom := tokenIn.Denom, tokenDenomOut

	// Get the expected order directionIn
	directionIn, err := r.OrderbookData.GetDirection(baseDenom, quoteDenom)
	if err != nil {
		return osmomath.BigDec{}, err
	}

	directionOut := directionIn.Opposite()

	spotPrice := osmomath.ZeroBigDec()
	for _, tickPrice := range tickPrices {
		// spotPrice.AddMut(cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, *directionIn, cosmwasmpool.ROUND_DOWN))
		// spotPrice.AddMut(cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, directionIn.Opposite(), cosmwasmpool.ROUND_DOWN))
		if r.OrderbookData.BaseDenom == baseDenom && r.OrderbookData.QuoteDenom == quoteDenom {
			// We're in the same direction, spot price is the same as the tick price
			spotPrice.AddMut(cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, *directionIn, cosmwasmpool.ROUND_DOWN))
		} else {
			// In the opposite direction, we need to invert the tick price.
			// For example, orderbook with base denom TRX and quote denom USDC, and tick price is 10.
			// When quote token in is USDC and token out is TRX, the spot price is 0.1, because 1 TRX is 10 USDC
			// but we want spot price in terms of USDC, not TRX, thus we invert 0.1 to 10.
			// spotPrice.AddMut(tickPrice)
			spotPrice.AddMut(cosmwasmpool.OrderbookValueInOppositeDirection(oneBigDec, tickPrice, directionOut, cosmwasmpool.ROUND_DOWN))
		}
	}

	spotPrice.QuoMut(osmomath.NewBigDec(int64(len(tickPrices))))

	return spotPrice, nil
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
