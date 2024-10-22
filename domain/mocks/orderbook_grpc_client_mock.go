package mocks

import (
	"context"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
	orderbookgrpcclientdomain "github.com/osmosis-labs/sqs/domain/orderbook/grpcclient"
)

var _ orderbookgrpcclientdomain.OrderBookClient = (*OrderbookGRPCClientMock)(nil)

// OrderbookGRPCClientMock is a mock struct that implements orderbookplugindomain.OrderbookGRPCClient.
type OrderbookGRPCClientMock struct {
	GetOrdersByTickCb            func(ctx context.Context, contractAddress string, tick int64) (orderbookdomain.Orders, error)
	GetActiveOrdersCb            func(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error)
	GetTickUnrealizedCancelsCb   func(ctx context.Context, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error)
	FetchTickUnrealizedCancelsCb func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error)
	MockQueryTicksCb             func(ctx context.Context, contractAddress string, ticks []int64) ([]orderbookdomain.Tick, error)
	FetchTicksCb                 func(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error)
}

func (o *OrderbookGRPCClientMock) WithGetOrdersByTickCb(orders orderbookdomain.Orders, err error) {
	o.GetOrdersByTickCb = func(ctx context.Context, contractAddress string, tick int64) (orderbookdomain.Orders, error) {
		return orders, err
	}
}

func (o *OrderbookGRPCClientMock) GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) (orderbookdomain.Orders, error) {
	if o.GetOrdersByTickCb != nil {
		return o.GetOrdersByTickCb(ctx, contractAddress, tick)
	}

	return nil, nil
}

func (o *OrderbookGRPCClientMock) GetActiveOrders(ctx context.Context, contractAddress string, ownerAddress string) (orderbookdomain.Orders, uint64, error) {
	if o.GetActiveOrdersCb != nil {
		return o.GetActiveOrdersCb(ctx, contractAddress, ownerAddress)
	}

	return nil, 0, nil
}

func (o *OrderbookGRPCClientMock) GetTickUnrealizedCancels(ctx context.Context, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
	if o.GetTickUnrealizedCancelsCb != nil {
		return o.GetTickUnrealizedCancelsCb(ctx, contractAddress, tickIDs)
	}

	return nil, nil
}

func (o *OrderbookGRPCClientMock) FetchTickUnrealizedCancels(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookgrpcclientdomain.UnrealizedTickCancels, error) {
	if o.FetchTickUnrealizedCancelsCb != nil {
		return o.FetchTickUnrealizedCancelsCb(ctx, chunkSize, contractAddress, tickIDs)
	}

	return nil, nil
}

func (o *OrderbookGRPCClientMock) QueryTicks(ctx context.Context, contractAddress string, ticks []int64) ([]orderbookdomain.Tick, error) {
	if o.MockQueryTicksCb != nil {
		return o.MockQueryTicksCb(ctx, contractAddress, ticks)
	}

	return nil, nil
}

func (o *OrderbookGRPCClientMock) FetchTicks(ctx context.Context, chunkSize int, contractAddress string, tickIDs []int64) ([]orderbookdomain.Tick, error) {
	if o.FetchTicksCb != nil {
		return o.FetchTicksCb(ctx, chunkSize, contractAddress, tickIDs)
	}

	return nil, nil
}
