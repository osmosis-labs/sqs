package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"
	"github.com/osmosis-labs/sqs/sqsdomain"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/osmosis-labs/osmosis/osmomath"
)

const (
	QUOTE_DENOM = "quote"
	BASE_DENOM  = "base"
	MIN_TICK    = -108000000
	MAX_TICK    = 182402823
	// Tick Price = 2
	LARGE_POSITIVE_TICK int64 = 1000000
	// Tick Price = 0.5
	LARGE_NEGATIVE_TICK int64 = -5000000
)

func (s *RoutablePoolTestSuite) SetupRoutableOrderbookPool(
	tokenInDenom,
	tokenOutDenom string,
	nextBidTick, nextAskTick int64,
	ticks []cosmwasmpool.OrderbookTickIdAndState,
	takerFee osmomath.Dec,
) sqsdomain.RoutablePool {
	// TODO: replace this with orderbook, but this should work as mock for now
	cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tokenInDenom, tokenOutDenom})

	poolType := cosmwasmPool.GetType()

	mock := &mocks.MockRoutablePool{
		ChainPoolModel: cosmwasmPool.AsSerializablePool(),
		CosmWasmPoolModel: cosmwasmpool.NewCWPoolModel(
			"crates.io:sumtree-orderbook", "0.1.0",
			cosmwasmpool.CosmWasmPoolData{
				Orderbook: &cosmwasmpool.OrderbookData{
					QuoteDenom:  QUOTE_DENOM,
					BaseDenom:   BASE_DENOM,
					NextBidTick: nextBidTick,
					NextAskTick: nextAskTick,
					Ticks:       ticks,
				},
			},
		),
		PoolType: poolType,
		TakerFee: takerFee,
	}

	routablePool, err := pools.NewRoutablePool(mock, tokenOutDenom, takerFee, domain.CosmWasmPoolRouterConfig{
		OrderbookCodeIDs: map[uint64]struct{}{
			defaultPoolID: {},
		},
	}, domain.UnsetScalingFactorGetterCb)
	s.Require().NoError(err)

	return routablePool
}

func (s *RoutablePoolTestSuite) TestCalculateTokenOutByTokenIn_Orderbook() {
	tests := map[string]struct {
		tokenIn     sdk.Coin
		tokenOut    sdk.Coin
		nextBidTick int64
		nextAskTick int64
		ticks       []cosmwasmpool.OrderbookTickIdAndState
		expectError error
	}{
		"BID: simple swap": {
			tokenIn:     sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			nextBidTick: MIN_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTickIdAndState{
				{TickId: 0, TickState: cosmwasmpool.OrderbookTickState{
					BidValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.ZeroBigDec(),
					},
					AskValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.NewBigDec(100),
					},
				}},
			},
		},
		"BID: invalid partial fill": {
			tokenIn:     sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(0)),
			nextBidTick: MIN_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTickIdAndState{
				{TickId: 0, TickState: cosmwasmpool.OrderbookTickState{
					BidValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.ZeroBigDec(),
					},
					AskValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.NewBigDec(25),
					},
				}},
			},
			expectError: domain.OrderbookNotEnoughLiquidityToCompleteSwapError{
				PoolId:   defaultPoolID,
				AmountIn: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)),
			},
		},
		"BID: multi-tick/direction swap": {
			tokenIn: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)),
			// 100*1 (tick: 0) + 50*2 (tick: LARGE_POSITIVE_TICK) = 200
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(200)),
			nextBidTick: LARGE_POSITIVE_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTickIdAndState{
				{
					TickId: 0,
					TickState: cosmwasmpool.OrderbookTickState{
						BidValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(10),
						},
						AskValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(100),
						},
					},
				},
				{
					TickId: LARGE_POSITIVE_TICK,
					TickState: cosmwasmpool.OrderbookTickState{
						BidValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(10),
						},
						AskValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(100),
						},
					},
				},
			},
		},
		"ASK: simple swap": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			tokenOut:    sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTickIdAndState{
				{TickId: 0, TickState: cosmwasmpool.OrderbookTickState{
					BidValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.NewBigDec(100),
					},
					AskValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.ZeroBigDec(),
					},
				}},
			},
		},
		"ASK: invalid partial fill": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(0)),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTickIdAndState{
				{TickId: 0, TickState: cosmwasmpool.OrderbookTickState{
					BidValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.NewBigDec(25),
					},
					AskValues: cosmwasmpool.OrderbookTickValues{
						TotalAmountOfLiquidity: osmomath.ZeroBigDec(),
					},
				}},
			},
			expectError: domain.OrderbookNotEnoughLiquidityToCompleteSwapError{
				PoolId:   defaultPoolID,
				AmountIn: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			},
		},
		"ASK: multi-tick/direction swap": {
			tokenIn: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			// 25 at 0.5 tick price + 100 at 1 tick price = 125
			tokenOut:    sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(125)),
			nextBidTick: LARGE_POSITIVE_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTickIdAndState{
				{
					TickId: 0,
					TickState: cosmwasmpool.OrderbookTickState{
						BidValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(100),
						},
						AskValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(100),
						},
					},
				},
				{
					TickId: LARGE_POSITIVE_TICK,
					TickState: cosmwasmpool.OrderbookTickState{
						BidValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(25),
						},
						AskValues: cosmwasmpool.OrderbookTickValues{
							TotalAmountOfLiquidity: osmomath.NewBigDec(25),
						},
					},
				},
			},
		},
		"invalid: duplicate denom": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(125)),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTickIdAndState{},
			expectError: domain.OrderbookPoolMismatchError{
				PoolId:        defaultPoolID,
				TokenInDenom:  BASE_DENOM,
				TokenOutDenom: BASE_DENOM,
			},
		},
		"invalid: incorrect token in denom": {
			tokenIn:     sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(125)),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTickIdAndState{},
			expectError: domain.OrderbookPoolMismatchError{
				PoolId:        defaultPoolID,
				TokenInDenom:  INVALID_DENOM,
				TokenOutDenom: BASE_DENOM,
			},
		},
		"invalid: incorrect token out denom": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(125)),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTickIdAndState{},
			expectError: domain.OrderbookPoolMismatchError{
				PoolId:        defaultPoolID,
				TokenInDenom:  BASE_DENOM,
				TokenOutDenom: INVALID_DENOM,
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableOrderbookPool(tc.tokenIn.Denom, tc.tokenOut.Denom, tc.nextBidTick, tc.nextAskTick, tc.ticks, osmomath.ZeroDec())
			tokenOut, err := routablePool.CalculateTokenOutByTokenIn(context.TODO(), tc.tokenIn)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().Equal(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			s.Require().Equal(tc.tokenOut, tokenOut)
		})
	}
}

// TODO: test spot price
