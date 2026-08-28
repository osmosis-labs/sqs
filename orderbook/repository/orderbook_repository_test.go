package orderbookrepository_test

import (
	"strconv"
	"sync"
	"sync/atomic"
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

// defaultBlockHeight is the block the test ticks are treated as coming from.
// StoreTicks discards updates older than what is already published, so tests
// that only care about merge behaviour keep the height constant.
const defaultBlockHeight uint64 = 100

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
				repo.StoreTicks(tt.poolId, defaultBlockHeight, tickAdditions)
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

// TestStoreTicks_RejectsStaleBlocks verifies that an update from an older block
// cannot overwrite a tick a newer block already published. Orderbook pools are
// processed in one goroutine per pool per block, so a slow block N can finish
// after block N+1.
func (s *OrderBookUseCaseTestSuite) TestStoreTicks_RejectsStaleBlocks() {
	repo := orderbookrepository.New()

	newer := map[int64]orderbookdomain.OrderbookTick{
		1: {Tick: withTickID(1), TickState: orderbookdomain.TickState{
			AskValues: orderbookdomain.TickValues{TotalAmountOfLiquidity: "newer"},
		}},
	}
	older := map[int64]orderbookdomain.OrderbookTick{
		1: {Tick: withTickID(1), TickState: orderbookdomain.TickState{
			AskValues: orderbookdomain.TickValues{TotalAmountOfLiquidity: "older"},
		}},
	}

	// Block 200 lands first, then the slower block 199 finishes.
	repo.StoreTicks(defaultPoolID, 200, newer)
	repo.StoreTicks(defaultPoolID, 199, older)

	ticks, ok := repo.GetAllTicks(defaultPoolID)
	s.Require().True(ok)
	s.Require().Equal("newer", ticks[1].TickState.AskValues.TotalAmountOfLiquidity,
		"a stale block must not overwrite a newer tick value")

	// A genuinely newer block still applies.
	repo.StoreTicks(defaultPoolID, 201, older)
	ticks, ok = repo.GetAllTicks(defaultPoolID)
	s.Require().True(ok)
	s.Require().Equal("older", ticks[1].TickState.AskValues.TotalAmountOfLiquidity,
		"a newer block must apply even if it lowers the value")
}

// TestStoreTicks_SameHeightBatchesAccumulate verifies that two updates carrying
// the same height both land. A block's ticks are fetched in batches, so equal
// heights must not be treated as stale.
func (s *OrderBookUseCaseTestSuite) TestStoreTicks_SameHeightBatchesAccumulate() {
	repo := orderbookrepository.New()

	repo.StoreTicks(defaultPoolID, 300, map[int64]orderbookdomain.OrderbookTick{
		1: {Tick: withTickID(1)},
	})
	repo.StoreTicks(defaultPoolID, 300, map[int64]orderbookdomain.OrderbookTick{
		2: {Tick: withTickID(2)},
	})

	ticks, ok := repo.GetAllTicks(defaultPoolID)
	s.Require().True(ok)
	s.Require().Len(ticks, 2, "batches at the same height must accumulate")
}

// TestStoreTicks_ConcurrentDisjointWrites verifies that concurrent writers to
// the same pool do not lose each other's updates.
func (s *OrderBookUseCaseTestSuite) TestStoreTicks_ConcurrentDisjointWrites() {
	repo := orderbookrepository.New()

	const (
		writers        = 8
		ticksPerWriter = 50
	)

	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < ticksPerWriter; i++ {
				tickID := int64(w*ticksPerWriter + i)
				repo.StoreTicks(defaultPoolID, defaultBlockHeight, map[int64]orderbookdomain.OrderbookTick{
					tickID: {Tick: withTickID(tickID)},
				})
			}
		}(w)
	}
	wg.Wait()

	ticks, ok := repo.GetAllTicks(defaultPoolID)
	s.Require().True(ok)
	s.Require().Len(ticks, writers*ticksPerWriter, "no concurrent update may be lost")
}

// TestStoreTicks_ReadersObserveCompleteSnapshots verifies that a reader never
// sees a half-applied update. Every tick in a published snapshot carries the
// same marker, so a snapshot mixing two blocks is detectable.
func (s *OrderBookUseCaseTestSuite) TestStoreTicks_ReadersObserveCompleteSnapshots() {
	repo := orderbookrepository.New()

	const (
		tickCount = 200
		rounds    = 200
	)

	writeRound := func(height uint64) {
		update := make(map[int64]orderbookdomain.OrderbookTick, tickCount)
		marker := strconv.FormatUint(height, 10)
		for i := int64(0); i < tickCount; i++ {
			update[i] = orderbookdomain.OrderbookTick{
				Tick: withTickID(i),
				TickState: orderbookdomain.TickState{
					AskValues: orderbookdomain.TickValues{TotalAmountOfLiquidity: marker},
				},
			}
		}
		repo.StoreTicks(defaultPoolID, height, update)
	}

	writeRound(1)

	var (
		wg   sync.WaitGroup
		torn int64
		stop = make(chan struct{})
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}

			ticks, ok := repo.GetAllTicks(defaultPoolID)
			if !ok {
				continue
			}

			marker := ""
			for _, tick := range ticks {
				if marker == "" {
					marker = tick.TickState.AskValues.TotalAmountOfLiquidity
				} else if tick.TickState.AskValues.TotalAmountOfLiquidity != marker {
					atomic.AddInt64(&torn, 1)
				}
			}
		}
	}()

	for height := uint64(2); height <= rounds; height++ {
		writeRound(height)
	}
	close(stop)
	wg.Wait()

	s.Require().Zero(atomic.LoadInt64(&torn),
		"a reader must never observe a snapshot mixing two blocks")
}
