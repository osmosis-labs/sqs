package orderbookusecase

import (
	"context"
	"github.com/osmosis-labs/sqs/domain"
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// OrderBookEntry is an alias of orderBookEntry for testing purposes
func (o *OrderbookUseCaseImpl) CreateFormattedLimitOrder(
	poolID uint64,
	order orderbookdomain.Order,
	quoteAsset orderbookdomain.Asset,
	baseAsset orderbookdomain.Asset,
	orderbookAddress string,
) (orderbookdomain.LimitOrder, error) {
	return o.createFormattedLimitOrder(poolID, order, quoteAsset, baseAsset, orderbookAddress)
}

// SetProcessOrderBookActiveOrdersFunc is a setter for processOrderBookActiveOrdersFunc
func (o *OrderbookUseCaseImpl) SetProcessOrderBookActiveOrdersFunc(fn func(ctx context.Context, orderbook domain.CanonicalOrderBooksResult, address string) ([]orderbookdomain.LimitOrder, bool, error)) {
	o.processOrderBookActiveOrdersFunc = fn
}
