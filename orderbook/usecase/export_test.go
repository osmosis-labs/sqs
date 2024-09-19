package orderbookusecase

import (
	"context"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// SetFetchActiveOrdersEveryDuration overrides the fetchActiveOrdersDuration for testing purposes
func (o *OrderbookUseCaseImpl) SetFetchActiveOrdersEveryDuration(duration time.Duration) {
	fetchActiveOrdersDuration = duration
}

// CreateFormattedLimitOrder is an alias of createFormattedLimitOrder for testing purposes
func (o *OrderbookUseCaseImpl) CreateFormattedLimitOrder(
	poolID uint64,
	order orderbookdomain.Order,
	quoteAsset orderbookdomain.Asset,
	baseAsset orderbookdomain.Asset,
	orderbookAddress string,
) (orderbookdomain.LimitOrder, error) {
	return o.createFormattedLimitOrder(poolID, order, quoteAsset, baseAsset, orderbookAddress)
}

// ProcessOrderBookActiveOrders is an alias of processOrderBookActiveOrders for testing purposes
func (o *OrderbookUseCaseImpl) ProcessOrderBookActiveOrders(ctx context.Context, orderBook domain.CanonicalOrderBooksResult, ownerAddress string) ([]orderbookdomain.LimitOrder, bool, error) {
	return o.processOrderBookActiveOrders(ctx, orderBook, ownerAddress)
}
