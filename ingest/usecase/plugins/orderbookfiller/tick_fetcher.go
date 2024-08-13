package orderbookfiller

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
)

func (o *orderbookFillerIngestPlugin) fetchTicksForOrderbook(ctx context.Context, orderbook domain.CanonicalOrderBooksResult) error {
	orderBookPool, err := o.poolsUseCase.GetPool(orderbook.PoolID)
	if err != nil {
		return err
	}

	ticks := orderBookPool.GetSQSPoolModel().CosmWasmPoolModel.Data.Orderbook.Ticks

	orderResult := orderbookplugindomain.OrdersResponse{
		AskOrders: []orderbookplugindomain.Order{},
		BidOrders: []orderbookplugindomain.Order{},
	}
	for _, tick := range ticks {
		orders, err := o.orderbookCWAAPIClient.GetOrdersByTick(ctx, orderbook.ContractAddress, tick.TickId)
		if err != nil {
			continue
		}

		for _, order := range orders {
			// Process order
			if order.OrderDirection == "ask" {
				orderResult.AskOrders = append(orderResult.AskOrders, order)
			} else {
				orderResult.BidOrders = append(orderResult.BidOrders, order)
			}
		}
	}

	// Store the orderbook orders
	o.orderMapByPoolID.Store(orderbook.PoolID, orderResult)

	return nil
}
