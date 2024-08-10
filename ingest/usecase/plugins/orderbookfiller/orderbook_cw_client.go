package orderbookfiller

import (
	"context"

	cosmwasmdomain "github.com/osmosis-labs/sqs/domain/cosmwasm"
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbookplugin"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
)

// orderbookCWAPIClient is an implementation of OrderbookCWAPIClient.
type orderbookCWAPIClient struct {
	wasmClient wasmtypes.QueryClient
}

var _ orderbookplugindomain.OrderbookCWAPIClient = (*orderbookCWAPIClient)(nil)

// ordersByTick is a struct that represents the request payload for the orders_by_tick query.
type ordersByTick struct {
	Tick int64 `json:"tick_id"`
}

// ordersByTickPayload is a struct that represents the payload for the orders_by_tick query.
type ordersByTickPayload struct {
	OrdersByTick ordersByTick `json:"orders_by_tick"`
}

// ordersByTickResponse is a struct that represents the response payload for the orders_by_tick query.
type ordersByTickResponse struct {
	Orders []orderbookplugindomain.Order `json:"orders"`
}

// NewOrderbookCWAPIClient creates a new orderbookCWAPIClient.
func NewOrderbookCWAPIClient(wasmClient wasmtypes.QueryClient) *orderbookCWAPIClient {
	return &orderbookCWAPIClient{
		wasmClient: wasmClient,
	}
}

// GetOrdersByTick implements OrderbookCWAPIClient.
func (o *orderbookCWAPIClient) GetOrdersByTick(ctx context.Context, contractAddress string, tick int64) ([]orderbookplugindomain.Order, error) {
	ordersByTick := ordersByTick{Tick: tick}

	var orders ordersByTickResponse
	if err := cosmwasmdomain.QueryCosmwasmContract(ctx, o.wasmClient, contractAddress, ordersByTickPayload{OrdersByTick: ordersByTick}, &orders); err != nil {
		return nil, err
	}

	return orders.Orders, nil
}
