package orderbookusecase

import (
	"context"
	"fmt"

	cwpoolmodel "github.com/osmosis-labs/osmosis/v25/x/cosmwasmpool/model"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/sqsdomain"
)

type orderbookUseCaseImpl struct {
	orderbookRepository orderbookdomain.OrderBookRepository
	orderBookClient     orderbookgrpcclientdomain.OrderBookClient
	logger              log.Logger
}

var _ mvc.OrderBookUsecase = &orderbookUseCaseImpl{}

func New(orderbookRepository orderbookdomain.OrderBookRepository, orderBookClient orderbookgrpcclientdomain.OrderBookClient, logger log.Logger) mvc.OrderBookUsecase {
	return &orderbookUseCaseImpl{
		orderbookRepository: orderbookRepository,
		orderBookClient:     orderBookClient,
		logger:              logger,
	}
}

// GetTicks implements mvc.OrderBookUsecase.
func (o *orderbookUseCaseImpl) GetAllTicks(poolID uint64) (map[int64]orderbookdomain.OrderbookTick, bool) {
	return o.orderbookRepository.GetAllTicks(poolID)
}

// StoreTicks implements mvc.OrderBookUsecase.
func (o *orderbookUseCaseImpl) ProcessPool(ctx context.Context, pool sqsdomain.PoolI) error {
	cosmWasmPoolModel := pool.GetSQSPoolModel().CosmWasmPoolModel
	if cosmWasmPoolModel == nil {
		return fmt.Errorf("cw pool model is nil when processing order book")
	}

	poolID := pool.GetId()
	if !cosmWasmPoolModel.IsOrderbook() {
		return fmt.Errorf("pool is not an orderbook pool %d", poolID)
	}

	// Update the orderbook client with the orderbook pool ID.
	ticks := cosmWasmPoolModel.Data.Orderbook.Ticks

	if len(ticks) > 0 {
		cwModel, ok := pool.GetUnderlyingPool().(*cwpoolmodel.CosmWasmPool)
		if !ok {
			return fmt.Errorf("failed to cast pool model to CosmWasmPool")
		}

		// Get tick IDs
		tickIDs := make([]int64, 0, len(ticks))
		for _, tick := range ticks {
			tickIDs = append(tickIDs, tick.TickId)
		}

		unrealizedCancels, err := o.orderBookClient.GetTickUnrealizedCancels(ctx, cwModel.ContractAddress, tickIDs)
		if err != nil {
			return fmt.Errorf("failed to fetch unrealized cancels for ticks %v: %w", tickIDs, err)
		}

		tickDataMap := make(map[int64]orderbookdomain.OrderbookTick, len(ticks))
		for i, tick := range ticks {
			unrealizedCancel := unrealizedCancels[i]

			// Validate the tick IDs match between the tick and the unrealized cancel
			if unrealizedCancel.TickID != tick.TickId {
				return fmt.Errorf("tick id mismatch when fetching unrealized ticks %d %d", unrealizedCancel.TickID, tick.TickId)
			}

			// Update tick map for the pool
			tickDataMap[tick.TickId] = orderbookdomain.OrderbookTick{
				Tick:              &ticks[i],
				UnrealizedCancels: unrealizedCancel.UnrealizedCancelsState,
			}
		}

		// Store the ticks
		o.orderbookRepository.StoreTicks(poolID, tickDataMap)
	}

	return nil
}
