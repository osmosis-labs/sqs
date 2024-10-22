package orderbookgrpcclientdomain

import (
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
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
	Orders []orderbookdomain.Order `json:"orders"`
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
	Ticks []UnrealizedTickCancels `json:"ticks"`
}

// UnrealizedTickCancels is a struct that represents the response payload from orderbook for unnrealized cancels by tick.
type UnrealizedTickCancels struct {
	TickID                 int64                             `json:"tick_id"`
	UnrealizedCancelsState orderbookdomain.UnrealizedCancels `json:"unrealized_cancels"`
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
	Orders orderbookdomain.Orders `json:"orders"`
	Count  uint64                 `json:"count"`
}

// ticksByID is a struct that represents the request payload for the queryTicksRequest query.
type ticksByID struct {
	TickIDs []int64 `json:"tick_ids"`
}

// queryTicksRequest is a struct that represents the payload for the QueryTicks query.
type queryTicksRequest struct {
	TicksByID ticksByID `json:"ticks_by_id"`
}

// queryTicksResponse is a struct that represents the response payload for the QueryTicks query.
type queryTicksResponse struct {
	Ticks []orderbookdomain.Tick `json:"ticks"`
}
