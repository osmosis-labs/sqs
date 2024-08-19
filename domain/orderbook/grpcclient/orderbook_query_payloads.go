package orderbookgrpcclientdomain

import (
	orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"
)

// ordersByTick is a struct that represents the request payload for the orders_by_tick query.
type ordersByTick struct {
	Tick int64 `json:"tick_id"`
}

// ordersByTickRequest is a struct that represents the payload for the orders_by_tick query.
type ordersByTickRequest struct {
	OrdersByTick ordersByTick `json:"orders_by_tick"`
}

// ordersByTickResponse is a struct that represents the response payload for the orders_by_tick query.
type ordersByTickResponse struct {
	Orders []orderbookplugindomain.Order `json:"orders"`
}

// unrealizedCancelsRequestPayload is a struct that represents the payload for the get_unrealized_cancels query.
type unrealizedCancelsRequestPayload struct {
	TickIds []int64 `json:"tick_ids"`
}

// unrealizedCancelsRequest is a struct that represents the payload for the get_unrealized_cancels query.
type unrealizedCancelsByTickIdRequest struct {
	UnrealizedCancels unrealizedCancelsRequestPayload `json:"get_unrealized_cancels"`
}

// unrealizedCancelsResponse is a struct that represents the response payload for the get_unrealized_cancels query.
type unrealizedCancelsResponse struct {
	Ticks []orderbookplugindomain.UnrealizedTickCancels `json:"ticks"`
}

// ordersByOwner is a struct that represents the request payload for the active_orders query.
type ordersByOwner struct {
	Owner string `json:"owner"`
}

// activeOrdersRequest is a struct that represents the payload for the active_orders query.
type activeOrdersRequest struct {
	OrdersByOwner ordersByOwner `json:"orders_by_owner"`
}

// activeOrdersResponse is a struct that represents the response payload for the active_orders query.
type activeOrdersResponse struct {
	Orders []orderbookplugindomain.Order `json:"orders"`
	Count  uint64                        `json:"count"`
}
