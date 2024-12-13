package pools_test

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	"github.com/osmosis-labs/sqs/domain"
	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/router/usecase/pools"

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
	nextBidTickIndex, nextAskTickIndex int,
	ticks []cosmwasmpool.OrderbookTick,
	takerFee osmomath.Dec,
) domain.RoutablePool {
	// TODO: replace this with orderbook, but this should work as mock for now
	cosmwasmPool := s.PrepareCustomTransmuterPool(s.TestAccs[0], []string{tokenInDenom, tokenOutDenom})

	poolType := cosmwasmPool.GetType()

	bidAmountToExhaustAskLiquidity, err := cosmwasmpool.CalcAmountInToExhaustOrderbookLiquidity(cosmwasmpool.BID, nextAskTickIndex, ticks)
	s.Require().NoError(err)

	askAmountToExhaustBidLiquidity, err := cosmwasmpool.CalcAmountInToExhaustOrderbookLiquidity(cosmwasmpool.ASK, nextBidTickIndex, ticks)
	s.Require().NoError(err)

	mock := &mocks.MockRoutablePool{
		ChainPoolModel: cosmwasmPool.AsSerializablePool(),
		CosmWasmPoolModel: cosmwasmpool.NewCWPoolModel(
			cosmwasmpool.ORDERBOOK_CONTRACT_NAME, cosmwasmpool.ORDERBOOK_MIN_CONTRACT_VERSION,
			cosmwasmpool.CosmWasmPoolData{
				Orderbook: &cosmwasmpool.OrderbookData{
					QuoteDenom:                     QUOTE_DENOM,
					BaseDenom:                      BASE_DENOM,
					NextBidTickIndex:               nextBidTickIndex,
					NextAskTickIndex:               nextAskTickIndex,
					BidAmountToExhaustAskLiquidity: bidAmountToExhaustAskLiquidity,
					AskAmountToExhaustBidLiquidity: askAmountToExhaustBidLiquidity,
					Ticks:                          ticks,
				},
			},
		),
		PoolType: poolType,
		TakerFee: takerFee,
	}

	cosmWasmPoolsParams := cosmwasmdomain.CosmWasmPoolsParams{
		Config: domain.CosmWasmPoolRouterConfig{
			OrderbookCodeIDs: map[uint64]struct{}{
				cosmwasmPool.GetId(): {},
			},
		},
		ScalingFactorGetterCb: domain.UnsetScalingFactorGetterCb,
	}
	routablePool, err := pools.NewRoutablePool(mock, tokenOutDenom, takerFee, cosmWasmPoolsParams)
	s.Require().NoError(err)

	return routablePool
}

func (s *RoutablePoolTestSuite) TestCalculateTokenOutByTokenIn_Orderbook() {
	tests := map[string]struct {
		tokenIn          sdk.Coin
		expectedTokenOut sdk.Coin
		nextBidTickIndex int
		nextAskTickIndex int
		ticks            []cosmwasmpool.OrderbookTick
		expectError      error
	}{
		"BID: simple swap": {
			tokenIn:          sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			nextBidTickIndex: MIN_TICK,
			nextAskTickIndex: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
		"BID: invalid partial fill": {
			tokenIn:          sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)),
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(0)),
			nextBidTickIndex: MIN_TICK,
			nextAskTickIndex: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(25),
				}},
			},
			expectError: domain.OrderbookNotEnoughLiquidityToCompleteSwapError{
				PoolId:   defaultPoolID,
				AmountIn: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)).String(),
			},
		},
		"BID: multi-tick/direction swap": {
			tokenIn: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(150)),
			// 100 / 1 (tick: 0) + 50/2 (tick: LARGE_POSITIVE_TICK) = 125
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(125)),
			nextBidTickIndex: -1, // no next bid tick
			nextAskTickIndex: 0,
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
		"BID: error not enough liquidity": {
			tokenIn:          sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			nextBidTickIndex: -1, // no next bid tick
			nextAskTickIndex: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(99),
				}},
			},
			expectError: domain.OrderbookNotEnoughLiquidityToCompleteSwapError{
				PoolId:   defaultPoolID,
				AmountIn: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)).String(),
			},
		},
		"ASK: simple swap": {
			tokenIn:          sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			expectedTokenOut: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			nextBidTickIndex: 0,
			nextAskTickIndex: -1, // no next ask tick
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
		},
		"ASK: invalid partial fill": {
			tokenIn:          sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			expectedTokenOut: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(0)),
			nextBidTickIndex: 0,
			nextAskTickIndex: -1, // no next ask tick
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(25),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
			expectError: domain.OrderbookNotEnoughLiquidityToCompleteSwapError{
				PoolId:   defaultPoolID,
				AmountIn: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)).String(),
			},
		},
		"ASK: multi-tick/direction swap": {
			tokenIn: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			// 50 at 2 tick price + 100 at 1 tick price = 200
			expectedTokenOut: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(200)),
			nextBidTickIndex: 1,
			nextAskTickIndex: 0,
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
						BidLiquidity: osmomath.NewBigDec(100),
						AskLiquidity: osmomath.NewBigDec(100),
					},
				},
			},
		},
		"ASK: error not enough liquidity": {
			tokenIn:          sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			expectedTokenOut: sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			nextBidTickIndex: 0,
			nextAskTickIndex: -1, // no next ask tick
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(99),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
			expectError: domain.OrderbookNotEnoughLiquidityToCompleteSwapError{
				PoolId:   defaultPoolID,
				AmountIn: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)).String(),
			},
		},
		"invalid: duplicate denom": {
			tokenIn:          sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(125)),
			nextBidTickIndex: -1, // no next bid tick
			nextAskTickIndex: -1, // no next ask tick
			ticks:            []cosmwasmpool.OrderbookTick{},
			expectError: cosmwasmpool.DuplicatedDenomError{
				Denom: BASE_DENOM,
			},
		},
		"invalid: incorrect token in denom": {
			tokenIn:          sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(150)),
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(125)),
			nextBidTickIndex: -1, // no next bid tick
			nextAskTickIndex: -1, // no next ask tick
			ticks:            []cosmwasmpool.OrderbookTick{},
			expectError: cosmwasmpool.OrderbookUnsupportedDenomError{
				Denom:      INVALID_DENOM,
				BaseDenom:  BASE_DENOM,
				QuoteDenom: QUOTE_DENOM,
			},
		},
		"invalid: incorrect token out denom": {
			tokenIn:          sdk.NewCoin(BASE_DENOM, osmomath.NewInt(150)),
			expectedTokenOut: sdk.NewCoin(INVALID_DENOM, osmomath.NewInt(125)),
			nextBidTickIndex: -1, // no next bid tick
			nextAskTickIndex: -1, // no next ask tick
			ticks:            []cosmwasmpool.OrderbookTick{},
			expectError: cosmwasmpool.OrderbookUnsupportedDenomError{
				Denom:      INVALID_DENOM,
				BaseDenom:  BASE_DENOM,
				QuoteDenom: QUOTE_DENOM,
			},
		},
		"error: overrid ": {
			tokenIn:          sdk.NewCoin(QUOTE_DENOM, osmomath.NewInt(100)),
			expectedTokenOut: sdk.NewCoin(BASE_DENOM, osmomath.NewInt(100)),
			nextBidTickIndex: MIN_TICK,
			nextAskTickIndex: 0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableOrderbookPool(tc.tokenIn.Denom, tc.expectedTokenOut.Denom, tc.nextBidTickIndex, tc.nextAskTickIndex, tc.ticks, osmomath.ZeroDec())
			tokenOut, err := routablePool.CalculateTokenOutByTokenIn(context.TODO(), tc.tokenIn)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().Equal(err, tc.expectError)
				return
			}
			s.Require().NoError(err)

			s.Require().Equal(tc.expectedTokenOut, tokenOut)
		})
	}
}

func (s *RoutablePoolTestSuite) TestCalcSpotPrice_Orderbook() {
	tests := map[string]struct {
		quoteDenom        string
		baseDenom         string
		expectedSpotPrice osmomath.BigDec
		nextBidTickIndex  int
		nextAskTickIndex  int
		ticks             []cosmwasmpool.OrderbookTick
		expectError       error
	}{
		"BID: basic price 1 query": {
			baseDenom:         QUOTE_DENOM,
			quoteDenom:        BASE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(1),
			nextBidTickIndex:  -1, // no next bid tick
			nextAskTickIndex:  0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.ZeroBigDec(),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
		"BID: multi tick lowest price": {
			baseDenom:         QUOTE_DENOM,
			quoteDenom:        BASE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(1),
			nextBidTickIndex:  -1, // no next bid tick
			nextAskTickIndex:  0,
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
			baseDenom:         QUOTE_DENOM,
			quoteDenom:        BASE_DENOM,
			expectedSpotPrice: osmomath.NewBigDecWithPrec(5, 1),
			nextBidTickIndex:  -1, // no next bid tick
			nextAskTickIndex:  1,
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
			baseDenom:         BASE_DENOM,
			quoteDenom:        QUOTE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(1),
			nextBidTickIndex:  0,
			nextAskTickIndex:  -1, // no next ask tick
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.ZeroBigDec(),
				}},
			},
		},
		"ASK: multi tick lowest price": {
			baseDenom:         BASE_DENOM,
			quoteDenom:        QUOTE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(1),
			nextBidTickIndex:  2,
			nextAskTickIndex:  -1, // no next ask tick
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
			baseDenom:         BASE_DENOM,
			quoteDenom:        QUOTE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(1),
			nextBidTickIndex:  0,
			nextAskTickIndex:  0,
			ticks: []cosmwasmpool.OrderbookTick{
				{TickId: 0, TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
					BidLiquidity: osmomath.NewBigDec(100),
					AskLiquidity: osmomath.NewBigDec(100),
				}},
			},
		},
		"ASK: change in spot price": {
			baseDenom:         BASE_DENOM,
			quoteDenom:        QUOTE_DENOM,
			expectedSpotPrice: osmomath.NewBigDecWithPrec(5, 1),
			nextBidTickIndex:  1,
			nextAskTickIndex:  -1, // no next ask tick
			ticks: []cosmwasmpool.OrderbookTick{
				{
					TickId: LARGE_NEGATIVE_TICK - 1,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.NewBigDec(100),
						AskLiquidity: osmomath.ZeroBigDec(),
					},
				},
				{
					TickId: LARGE_NEGATIVE_TICK,
					TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
						BidLiquidity: osmomath.NewBigDec(100),
						AskLiquidity: osmomath.ZeroBigDec(),
					},
				},
			},
		},
		"invalid: duplicate denom": {
			quoteDenom:        BASE_DENOM,
			baseDenom:         BASE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(0),
			nextBidTickIndex:  -1, // no next bid tick
			nextAskTickIndex:  -1, // no next ask tick
			ticks:             []cosmwasmpool.OrderbookTick{},
			expectError: cosmwasmpool.DuplicatedDenomError{
				Denom: BASE_DENOM,
			},
		},
		"invalid: incorrect base denom": {
			baseDenom:         INVALID_DENOM,
			quoteDenom:        QUOTE_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(0),
			nextBidTickIndex:  -1, // no next bid tick
			nextAskTickIndex:  -1, // no next ask tick
			ticks:             []cosmwasmpool.OrderbookTick{},
			expectError: cosmwasmpool.OrderbookUnsupportedDenomError{
				Denom:      INVALID_DENOM,
				BaseDenom:  BASE_DENOM,
				QuoteDenom: QUOTE_DENOM,
			},
		},
		"invalid: incorrect quote denom": {
			baseDenom:         BASE_DENOM,
			quoteDenom:        INVALID_DENOM,
			expectedSpotPrice: osmomath.NewBigDec(0),
			nextBidTickIndex:  -1, // no next bid tick
			nextAskTickIndex:  -1, // no next ask tick
			ticks:             []cosmwasmpool.OrderbookTick{},
			expectError: cosmwasmpool.OrderbookUnsupportedDenomError{
				Denom:      INVALID_DENOM,
				BaseDenom:  BASE_DENOM,
				QuoteDenom: QUOTE_DENOM,
			},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableOrderbookPool(tc.baseDenom, tc.quoteDenom, tc.nextBidTickIndex, tc.nextAskTickIndex, tc.ticks, osmomath.ZeroDec())
			spotPrice, err := routablePool.CalcSpotPrice(context.TODO(), tc.baseDenom, tc.quoteDenom)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().Equal(err, tc.expectError)
				return
			}
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedSpotPrice, spotPrice)
		})
	}
}

func (s *RoutablePoolTestSuite) TestGetDirection() {
	tests := map[string]struct {
		tokenInDenom      string
		tokenOutDenom     string
		expectedDirection cosmwasmpool.OrderbookDirection
		expectError       error
	}{
		"BID direction": {
			tokenInDenom:      QUOTE_DENOM,
			tokenOutDenom:     BASE_DENOM,
			expectedDirection: cosmwasmpool.BID,
		},
		"ASK direction": {
			tokenInDenom:      BASE_DENOM,
			tokenOutDenom:     QUOTE_DENOM,
			expectedDirection: cosmwasmpool.ASK,
		},
		"invalid direction": {
			tokenInDenom:  "invalid",
			tokenOutDenom: BASE_DENOM,
			expectError:   cosmwasmpool.OrderbookUnsupportedDenomError{Denom: "invalid", BaseDenom: BASE_DENOM, QuoteDenom: QUOTE_DENOM},
		},
	}

	for name, tc := range tests {
		s.Run(name, func() {
			s.Setup()
			routablePool := s.SetupRoutableOrderbookPool(tc.tokenInDenom, tc.tokenOutDenom, MIN_TICK, MAX_TICK, nil, osmomath.ZeroDec())

			routableOrderbookPool, ok := routablePool.(*pools.RoutableOrderbookPoolImpl)

			if !ok {
				s.FailNow("failed to cast to RouteableOrderbookPoolImpl")
			}

			direction, err := routableOrderbookPool.OrderbookData.GetDirection(tc.tokenInDenom, tc.tokenOutDenom)

			if tc.expectError != nil {
				s.Require().Error(err)
				s.Require().Equal(err, tc.expectError)
				return
			}
			s.Require().NoError(err)
			s.Require().Equal(tc.expectedDirection, *direction)
		})
	}
}
