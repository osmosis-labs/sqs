package orderbookplugindomain

import orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

// UnrealizedTickCancels is a struct that represents the response payload from orderbook for unnrealized cancels by tick.
type UnrealizedTickCancels struct {
	TickID                 int64                             `json:"tick_id"`
	UnrealizedCancelsState orderbookdomain.UnrealizedCancels `json:"unrealized_cancels"`
}
