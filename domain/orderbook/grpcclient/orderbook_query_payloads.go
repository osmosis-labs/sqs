package orderbookgrpcclientdomain

import orderbookplugindomain "github.com/osmosis-labs/sqs/domain/orderbook/plugin"

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
