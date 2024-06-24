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
	ticks []cosmwasmpool.OrderbookTick,
	takerFee osmomath.Dec,
) sqsdomain.RoutablePool {
	// TODO: replace this with orderbook, but this should work as mock for now
	cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tokenInDenom, tokenOutDenom})

	poolType := cosmwasmPool.GetType()

	mock := &mocks.MockRoutablePool{
		ChainPoolModel: cosmwasmPool.AsSerializablePool(),
		CosmWasmPoolModel: cosmwasmpool.NewCWPoolModel(
			cosmwasmpool.ORDERBOOK_CONTRACT_NAME, cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
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
			cosmwasmPool.GetId(): {},
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
		ticks       []cosmwasmpool.OrderbookTick
		expectError error
	}{
		"BID: simple swap": {
			tokenIn:     sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			nextBidTick: MIN_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
		"BID: invalid partial fill": {
			tokenIn:     sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(0)),
			nextBidTick: MIN_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(25),
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
			ticks: []cosmwasmpool.OrderbookTick{
				{
					TickId: 0,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
				{
					TickId: LARGE_POSITIVE_TICK,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
			},
		},
		"ASK: simple swap": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			tokenOut:    sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
		},
		"ASK: invalid partial fill": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(0)),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(25),
					AskLiquidity: osmomath.ZeroBigDec(),
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
			ticks: []cosmwasmpool.OrderbookTick{
				{
					TickId: 0,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.NewBigDec(100),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
				{
					TickId: LARGE_POSITIVE_TICK,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.NewBigDec(25),
						AskLiquidity: osmomath.NewBigDec(25),
					},
				},
			},
		},
		"invalid: duplicate denom": {
			tokenIn:     sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			tokenOut:    sdk.NewCoin(BASE_DENOM, osmomath.NewInt(125)),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTick{},
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
			ticks:       []cosmwasmpool.OrderbookTick{},
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
			ticks:       []cosmwasmpool.OrderbookTick{},
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

func (s *RoutablePoolTestSuite) TestCalcSpotPrice_Orderbook() {
	tests := map[string]struct {
		quoteDenom  string
		baseDenom   string
		spotPrice   osmomath.BigDec
		nextBidTick int64
		nextAskTick int64
		ticks       []cosmwasmpool.OrderbookTick
		expectError error
	}{
		"BID: basic price 1 query": {
			baseDenom:   BASE_DENOM,
			quoteDenom:  QUOTE_DENOM,
			spotPrice:   osmomath.NewBigDec(1),
			nextBidTick: MIN_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
		"BID: multi tick lowest price": {
			baseDenom:   BASE_DENOM,
			quoteDenom:  QUOTE_DENOM,
			spotPrice:   osmomath.NewBigDec(1),
			nextBidTick: MIN_TICK,
			nextAskTick: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{
					TickId: 0,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
				{
					TickId: 1,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
				{
					TickId: 2,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
			},
		},
		"BID: change in spot price": {
			baseDenom:   BASE_DENOM,
			quoteDenom:  QUOTE_DENOM,
			spotPrice:   osmomath.NewBigDec(2),
			nextBidTick: MIN_TICK,
			nextAskTick: LARGE_POSITIVE_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{
					TickId: 0,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
				{
					TickId: LARGE_POSITIVE_TICK,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.ZeroBigDec(),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
			},
		},
		"ASK: basic price 1 query": {
			baseDenom:   QUOTE_DENOM,
			quoteDenom:  BASE_DENOM,
			spotPrice:   osmomath.NewBigDec(1),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
		},
		"ASK: multi tick lowest price": {
			baseDenom:   QUOTE_DENOM,
			quoteDenom:  BASE_DENOM,
			spotPrice:   osmomath.NewBigDec(1),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: -2, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
				{TickId: -1, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
		},
		"ASK: multi direction lowest tick": {
			baseDenom:   QUOTE_DENOM,
			quoteDenom:  BASE_DENOM,
			spotPrice:   osmomath.NewBigDec(1),
			nextBidTick: 0,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
		"ASK: change in spot price": {
			baseDenom:   QUOTE_DENOM,
			quoteDenom:  BASE_DENOM,
			spotPrice:   osmomath.NewBigDecWithPrec(5, 1),
			nextBidTick: LARGE_NEGATIVE_TICK,
			nextAskTick: MAX_TICK,
			ticks: []cosmwasmpool.OrderbookTick{
				{
					TickId: LARGE_NEGATIVE_TICK,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.NewBigDec(100),
						AskLiquidity: osmomath.ZeroBigDec(),
					},
				},
				{
					TickId: 0,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.NewBigDec(100),
						AskLiquidity: osmomath.ZeroBigDec(),
					},
				},
			},
		},
		"invalid: duplicate denom": {
			quoteDenom:  BASE_DENOM,
			baseDenom:   BASE_DENOM,
			spotPrice:   osmomath.NewBigDec(0),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTick{},
			expectError: domain.OrderbookPoolMismatchError{
				PoolId:        defaultPoolID,
				TokenInDenom:  BASE_DENOM,
				TokenOutDenom: BASE_DENOM,
			},
		},
		"invalid: incorrect base denom": {
			baseDenom:   INVALID_DENOM,
			quoteDenom:  QUOTE_DENOM,
			spotPrice:   osmomath.NewBigDec(0),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTick{},
			expectError: domain.OrderbookPoolMismatchError{
				PoolId:        defaultPoolID,
				TokenInDenom:  QUOTE_DENOM,
				TokenOutDenom: INVALID_DENOM,
			},
		},
		"invalid: incorrect quote denom": {
			baseDenom:   BASE_DENOM,
			quoteDenom:  INVALID_DENOM,
			spotPrice:   osmomath.NewBigDec(0),
			nextBidTick: 0,
			nextAskTick: 0,
			ticks:       []cosmwasmpool.OrderbookTick{},
			expectError: domain.OrderbookPoolMismatchError{
				PoolId:        defaultPoolID,
				TokenInDenom:  INVALID_DENOM,
				TokenOutDenom: BASE_DENOM,
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableOrderbookPool(tc.quoteDenom, tc.baseDenom, tc.nextBidTick, tc.nextAskTick, tc.ticks, osmomath.ZeroDec())
			spotPrice, err := routablePool.CalcSpotPrice(context.TODO(), tc.baseDenom, tc.quoteDenom)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().Equal(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			s.Require().Equal(tc.spotPrice, spotPrice)
		})
	}
}

func (s *RoutablePoolTestSuite) TestGetDirection() {
	tests := map[string]struct {
		tokenInDenom  string
		tokenOutDenom string
		expected      domain.OrderbookDirection
		expectError   error
	}{
		"BID direction": {
			tokenInDenom:  QUOTE_DENOM,
			tokenOutDenom: BASE_DENOM,
			expected:      domain.BID,
		},
		"ASK direction": {
			tokenInDenom:  BASE_DENOM,
			tokenOutDenom: QUOTE_DENOM,
			expected:      domain.ASK,
		},
		"invalid direction": {
			tokenInDenom:  "invalid",
			tokenOutDenom: BASE_DENOM,
			expected:      0,
			expectError:   domain.OrderbookPoolMismatchError{PoolId: defaultPoolID, TokenInDenom: "invalid", TokenOutDenom: BASE_DENOM},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableOrderbookPool(tc.tokenInDenom, tc.tokenOutDenom, MIN_TICK, MAX_TICK, nil, osmomath.ZeroDec())

			routableOrderbookPool, ok := routablePool.(*pools.RouteableOrderbookPoolImpl)

			if !ok {
				s.FailNow("failed to cast to RouteableOrderbookPoolImpl")
			}

			direction, err := routableOrderbookPool.GetDirection(tc.tokenInDenom, tc.tokenOutDenom)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().Equal(err, tc.expectError)
				return
			}
			s.Require().NoError(err)
			s.Require().Equal(tc.expected, direction)
		})
	}
}
