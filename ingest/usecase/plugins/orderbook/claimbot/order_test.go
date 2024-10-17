package claimbot_test

import (
	"context"
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	"github.com/osmosis-labs/sqs/ingest/usecase/plugins/orderbook/claimbot"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"

	"github.com/stretchr/testify/assert"
)

func TestProcessOrderbooksAndGetClaimableOrders(t *testing.T) {
	newOrderbookTick := func(tickID int64) map[int64]orderbookdomain.OrderbookTick {
		return map[int64]orderbookdomain.OrderbookTick{
			tickID: {
				Tick: &cosmwasmpool.OrderbookTick{
					TickId: tickID,
				},
			},
		}
	}

	newOrderbookFullyFilledTick := func(tickID int64, direction string) map[int64]orderbookdomain.OrderbookTick {
		tick := orderbookdomain.OrderbookTick{
			Tick: &cosmwasmpool.OrderbookTick{
				TickId: tickID,
			},
			TickState: orderbookdomain.TickState{},
		}

		tickValue := orderbookdomain.TickValues{
			CumulativeTotalValue:        "100",
			EffectiveTotalAmountSwapped: "100",
		}

		if direction == "bid" {
			tick.TickState.BidValues = tickValue
		} else {
			tick.TickState.AskValues = tickValue
		}

		return map[int64]orderbookdomain.OrderbookTick{
			tickID: tick,
		}
	}

	newOrder := func(direction string) orderbookdomain.Order {
		return orderbookdomain.Order{
			TickId:         1,
			OrderId:        1,
			OrderDirection: direction,
		}
	}

	newLimitOrder := func(percentFilled osmomath.Dec) orderbookdomain.LimitOrder {
		return orderbookdomain.LimitOrder{
			OrderId:       1,
			PercentFilled: percentFilled,
		}
	}

	newCanonicalOrderBooksResult := func(poolID uint64, contractAddress string) domain.CanonicalOrderBooksResult {
		return domain.CanonicalOrderBooksResult{PoolID: poolID, ContractAddress: contractAddress}
	}

	tests := []struct {
		name           string
		fillThreshold  osmomath.Dec
		orderbooks     []domain.CanonicalOrderBooksResult
		mockSetup      func(*mocks.OrderbookRepositoryMock, *mocks.OrderbookGRPCClientMock, *mocks.OrderbookUsecaseMock)
		expectedOrders []claimbot.Order
	}{
		{
			name:          "No orderbooks",
			fillThreshold: osmomath.NewDec(1),
			orderbooks:    []domain.CanonicalOrderBooksResult{},
			mockSetup: func(repo *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, usecase *mocks.OrderbookUsecaseMock) {
				repo.GetAllTicksFunc = func(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
					return nil, false
				}
			},
			expectedOrders: nil,
		},
		{
			name:          "Single orderbook with no claimable orders",
			fillThreshold: osmomath.NewDecWithPrec(95, 2), // 0.95
			orderbooks: []domain.CanonicalOrderBooksResult{
				newCanonicalOrderBooksResult(10, "contract1"),
			},
			mockSetup: func(repository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, usecase *mocks.OrderbookUsecaseMock) {
				repository.WithGetAllTicksFunc(newOrderbookTick(1), true)

				client.WithGetOrdersByTickCb(orderbookdomain.Orders{
					newOrder("ask"),
				}, nil)

				// Not claimable order, below threshold
				usecase.WithCreateFormattedLimitOrder(newLimitOrder(osmomath.NewDecWithPrec(90, 2)), nil)
			},
			expectedOrders: []claimbot.Order{
				{
					Orderbook: newCanonicalOrderBooksResult(10, "contract1"), // orderbook with
					Orders:    nil,                                           // no claimable orders
				},
			},
		},
		{
			name:          "Tick fully filled: all orders are claimable",
			fillThreshold: osmomath.NewDecWithPrec(99, 2), // 0.99
			orderbooks: []domain.CanonicalOrderBooksResult{
				newCanonicalOrderBooksResult(38, "contract8"),
			},
			mockSetup: func(repository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, usecase *mocks.OrderbookUsecaseMock) {
				repository.WithGetAllTicksFunc(newOrderbookFullyFilledTick(35, "bid"), true)

				client.WithGetOrdersByTickCb(orderbookdomain.Orders{
					newOrder("bid"),
				}, nil)

				usecase.WithCreateFormattedLimitOrder(newLimitOrder(osmomath.NewDecWithPrec(90, 2)), nil)
			},
			expectedOrders: []claimbot.Order{
				{
					Orderbook: newCanonicalOrderBooksResult(38, "contract8"),
					Orders:    orderbookdomain.Orders{newOrder("bid")},
				},
			},
		},
		{
			name:          "Orderbook with claimable orders",
			fillThreshold: osmomath.NewDecWithPrec(95, 2), // 0.95
			orderbooks: []domain.CanonicalOrderBooksResult{
				newCanonicalOrderBooksResult(64, "contract58"),
			},
			mockSetup: func(repository *mocks.OrderbookRepositoryMock, client *mocks.OrderbookGRPCClientMock, usecase *mocks.OrderbookUsecaseMock) {
				repository.WithGetAllTicksFunc(newOrderbookTick(42), true)

				client.WithGetOrdersByTickCb(orderbookdomain.Orders{
					newOrder("ask"),
				}, nil)

				// Claimable order, above threshold
				usecase.WithCreateFormattedLimitOrder(newLimitOrder(osmomath.NewDecWithPrec(96, 2)), nil)
			},
			expectedOrders: []claimbot.Order{
				{
					Orderbook: newCanonicalOrderBooksResult(64, "contract58"),
					Orders:    orderbookdomain.Orders{newOrder("ask")},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			repository := mocks.OrderbookRepositoryMock{}
			client := mocks.OrderbookGRPCClientMock{}
			usecase := mocks.OrderbookUsecaseMock{}
			logger := log.NoOpLogger{}

			tt.mockSetup(&repository, &client, &usecase)

			result := claimbot.ProcessOrderbooksAndGetClaimableOrders(ctx, tt.fillThreshold, tt.orderbooks, &repository, &client, &usecase, &logger)

			assert.Equal(t, tt.expectedOrders, result)
		})
	}
}
