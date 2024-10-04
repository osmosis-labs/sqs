package orderbookplugindomain

import (
	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"
)

// OrdersResponse represents the response from the orderbook contract containing the orders for a given tick.
type OrdersResponse struct {
	Address   string                  `json:"address"`
	BidOrders []orderbookdomain.Order `json:"bid_orders"`
	AskOrders []orderbookdomain.Order `json:"ask_orders"`
}
