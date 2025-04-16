package orderbookorderscache

import (
	"context"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	passthroughdomain "github.com/osmosis-labs/sqs/domain/passthrough"
	"github.com/osmosis-labs/sqs/log"

	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

// ordersCache is a plugin that caches orderbook orders at the end of each block.
// Plugin fetches changed orderbook orders from the Node at each block and stores it for later efficient retrieval.
type ordersCache struct {
	config *Config
}

var _ domain.EndBlockProcessPlugin = &ordersCache{}

const (
	tracerName = "sqs-orderbook-orders-cache"
)

var (
	tracer = otel.Tracer(tracerName)
)

// New creates and returns a new order caching instance.
func New(
	orderbookRepository orderbookdomain.OrderBookRepository,
	poolsUsecase mvc.PoolsUsecase,
	logger log.Logger,
	passthroughGRPCClient passthroughdomain.PassthroughGRPCClient,
) *ordersCache {
	config := NewConfig(orderbookRepository, poolsUsecase, logger, passthroughGRPCClient)

	return &ordersCache{
		config: config,
	}
}

func (o *ordersCache) ProcessEndBlock(ctx context.Context, blockHeight uint64, metadata domain.BlockPoolMetadata) error {
	ctx, span := tracer.Start(ctx, "orderbookOrdersCacheIngestPlugin.ProcessEndBlock")
	defer span.End()

	orderbooks, err := getOrderbooks(o.config.PoolsUsecase, metadata)
	if err != nil {
		return err
	}

	for _, orderbook := range orderbooks {
		if _, ok := metadata.PoolIDs[orderbook.PoolID]; ok {
			if err := o.fetchAndCacheOrdersForOrderbook(ctx, orderbook); err != nil {
				o.config.Logger.Error("failed to fetch orders for orderbook", zap.Error(err), zap.Uint64("orderbook_id", orderbook.PoolID))
				return err
			}
		}
	}

	return nil
}

func (o *ordersCache) fetchAndCacheOrdersForOrderbook(ctx context.Context, orderbook domain.CanonicalOrderBooksResult) error {
	ordersBz, err := o.config.PassthroughGRPCClient.GetOrderbookOrdersRaw(ctx, orderbook.PoolID)
	if err != nil {
		return err
	}

	orders := make([]orderbookdomain.Order, 0, len(ordersBz))
	for _, orderBz := range ordersBz {
		var order orderbookdomain.Order
		if err := json.Unmarshal(orderBz, &order); err != nil {
			return err
		}

		orders = append(orders, order)
	}

	return o.config.OrderbookRepository.StoreOrders(orderbook.PoolID, orders)
}
