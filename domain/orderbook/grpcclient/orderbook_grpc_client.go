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

	// GetTickUnrealizedCancels fetches unrealized cancels by tick from the orderbook contract.
	GetTickUnrealizedCancels(ctx context.Context, contractAddress string, tickIDs []int64) ([]unrealizedCancelsTickPayload, error)
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
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, ordersByTickRequest{OrdersByTick: ordersByTick}, &orders); err != nil {
		return nil, err
	}

	return orders.Orders, nil
}

// GetTickUnrealizedCancels implements OrderbookCWAPIClient.
func (o *orderbookClientImpl) GetTickUnrealizedCancels(ctx context.Context, contractAddress string, tickIDs []int64) ([]unrealizedCancelsTickPayload, error) {
	var unrealizedCancels unrealizedCancelsResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, unrealizedCancelsByTickIdRequest{UnrealizedCancels: unrealizedCancelsRequestPayload{TickIds: tickIDs}}, &unrealizedCancels); err != nil {
		return nil, err
	}
	return unrealizedCancels.Ticks, nil
}
