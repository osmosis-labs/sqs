package orderbookgrpcclientdomain

import (
	"context"

	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

// OrderBookClient is an interface for fetching orders by tick from the orderbook contract.
type OrderBookClient interface {
	// GetOrdersByTick fetches orders by tick from the orderbook contract.
	GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) ([]orderbookplugindomain.Order, error)
}

// orderbookClientImpl is an implementation of OrderbookCWAPIClient.
type orderbookClientImpl struct {
	wasmClient wasmtypes.QueryClient
}

var _ OrderBookClient = (*orderbookClientImpl)(nil)

// New creates a new orderbookClientImpl.
func New(wasmClient wasmtypes.QueryClient) *orderbookClientImpl {
	return &orderbookClientImpl{
		wasmClient: wasmClient,
	}
}

// GetOrdersByTick implements OrderbookCWAPIClient.
func (o *orderbookClientImpl) GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) ([]orderbookplugindomain.Order, error) {
	ordersByTick := ordersByTick{Tick: tick}

	var orders ordersByTickResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, ordersByTickPayload{OrdersByTick: ordersByTick}, &orders); err != nil {
		return nil, err
	}

	return orders.Orders, nil
}
