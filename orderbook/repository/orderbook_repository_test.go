package orderbookrepository_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookrepository "github.com/osmosis-labs/sqs/orderbook/repository"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting"
	"github.com/stretchr/testify/suite"
)

type OrderBookUseCaseTestSuite struct {
	routertesting.RouterTestHelper
}

func TestOrderBookUseCase(t *testing.T) {
	suite.Run(t, new(OrderBookUseCaseTestSuite))
}

var (
	defaultTickLiquidity = osmomath.NewBigDec(1_000_000)
	doubleTickLiquidity  = osmomath.NewBigDec(2_000_000)

	defaultTick = &cosmwasmpool.OrderbookTick{
		TickId: 1,
		TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
			BidLiquidity: defaultTickLiquidity,
			AskLiquidity: doubleTickLiquidity,
		},
	}

	withTickID = func(tickID int64) *cosmwasmpool.OrderbookTick {
		return &cosmwasmpool.OrderbookTick{
			TickId: tickID,
			TickLiquidity: cosmwasmpool.OrderbookTickLiquidity{
				BidLiquidity: defaultTickLiquidity,
				AskLiquidity: doubleTickLiquidity,
			},
		}
	}

	defaultPoolID uint64 = 1

	defaultTicks = map[int64]orderbookdomain.OrderbookTick{
		1: {
			Tick: withTickID(1),
		},
		2: {
			Tick: withTickID(2),
		},
	}
)

func (s *OrderBookUseCaseTestSuite) TestStoreTicks() {
	tests := []struct {
		name          string
		poolId        uint64
		tickAdditions []map[int64]orderbookdomain.OrderbookTick

		expectedTicks map[int64]orderbookdomain.OrderbookTick
	}{
		{
			name:          "one addition",
			poolId:        defaultPoolID,
			tickAdditions: []map[int64]orderbookdomain.OrderbookTick{defaultTicks},

			expectedTicks: defaultTicks,
		},
		{
			name:   "two additions",
			poolId: defaultPoolID,
			tickAdditions: []map[int64]orderbookdomain.OrderbookTick{
				defaultTicks,
				{
					3: {
						Tick: withTickID(3),
					},
				},
			},

			expectedTicks: map[int64]orderbookdomain.OrderbookTick{
				1: {
					Tick: withTickID(1),
				},
				2: {
					Tick: withTickID(2),
				},
				3: {
					Tick: withTickID(3),
				},
			},
		},
		{
			name:   "empty ticks added",
			poolId: defaultPoolID,
			tickAdditions: []map[int64]orderbookdomain.OrderbookTick{
				map[int64]orderbookdomain.OrderbookTick{},
			},
			expectedTicks: map[int64]orderbookdomain.OrderbookTick{},
		},
		{
			name:          "no ticks added",
			poolId:        defaultPoolID,
			tickAdditions: []map[int64]orderbookdomain.OrderbookTick{},
		},
	}

	for _, tt := range tests {
		tt := tt
		s.T().Run(tt.name, func(t *testing.T) {

			repo := orderbookrepository.New()

			// System under test
			for _, tickAdditions := range tt.tickAdditions {
				repo.StoreTicks(tt.poolId, tickAdditions)
			}

			actualTicks, ok := repo.GetAllTicks(tt.poolId)

			// If no ticks were added, the ticks should not be found
			if len(tt.tickAdditions) == 0 {
				s.Require().False(ok)
				return
			}

			// If ticks were added, they should be found and equal to the expected ticks
			s.Require().True(ok)
			s.Require().Equal(tt.expectedTicks, actualTicks)
		})
	}
}
