package orderbookdomain

import (
	"fmt"
	"strconv"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// OrderStatus represents the status of an order.
type OrderStatus string

// Order status types.
const (
	StatusOpen            OrderStatus = "open"
	StatusPartiallyFilled OrderStatus = "partiallyFilled"
	StatusFilled          OrderStatus = "filled"
	StatusFullyClaimed    OrderStatus = "fullyClaimed"
	StatusCancelled       OrderStatus = "cancelled"
)

// Order represents an order in the orderbook returned by the orderbook contract.
type Order struct {
	TickId         int64  `json:"tick_id"`
	OrderId        int64  `json:"order_id"`
	OrderDirection string `json:"order_direction"`
	Owner          string `json:"owner"`
	Quantity       string `json:"quantity"`
	Etas           string `json:"etas"`
	ClaimBounty    string `json:"claim_bounty"`
	PlacedQuantity string `json:"placed_quantity"`
	PlacedAt       string `json:"placed_at"`
}

// Status returns the status of the order based on the percent filled.
func (o Order) Status(percentFilled float64) (OrderStatus, error) {
	quantity, err := strconv.Atoi(o.Quantity)
	if err != nil {
		return "", fmt.Errorf("error parsing quantity: %w", err)
	}

	if quantity == 0 || percentFilled == 1 {
		return StatusFilled, nil
	}

	if percentFilled == 0 {
		return StatusOpen, nil
	}

	if percentFilled < 1 {
		return StatusPartiallyFilled, nil
	}

	return StatusOpen, nil
}

// Orders represents a list of orders in the orderbook returned by the orderbook contract.
type Orders []Order

// TickID returns a list of tick IDs from the orders.
func (o Orders) TickID() []int64 {
	var tickIDs []int64
	for _, order := range o {
		tickIDs = append(tickIDs, order.TickId)
	}
	return tickIDs
}

// Asset represents orderbook asset returned by the orderbook contract.
type Asset struct {
	Symbol   string `json:"symbol"`
	Decimals int    `json:"-"`
}

// LimitOrder represents a limit order in the orderbook.
type LimitOrder struct {
	TickId           int64        `json:"tick_id"`
	OrderId          int64        `json:"order_id"`
	OrderDirection   string       `json:"order_direction"`
	Owner            string       `json:"owner"`
	Quantity         osmomath.Dec `json:"quantity"`
	Etas             string       `json:"etas"`
	ClaimBounty      string       `json:"claim_bounty"`
	PlacedQuantity   osmomath.Dec `json:"placed_quantity"`
	PlacedAt         int64        `json:"placed_at"`
	Price            osmomath.Dec `json:"price"`
	PercentClaimed   osmomath.Dec `json:"percentClaimed"`
	TotalFilled      osmomath.Dec `json:"totalFilled"`
	PercentFilled    osmomath.Dec `json:"percentFilled"`
	OrderbookAddress string       `json:"orderbookAddress"`
	Status           OrderStatus  `json:"status"`
	Output           osmomath.Dec `json:"output"`
	QuoteAsset       Asset        `json:"quote_asset"`
	BaseAsset        Asset        `json:"base_asset"`
	PlacedTx         *string      `json:"placed_tx,omitempty"`
}

// OrderbookResult represents orderbook orders result.
type OrderbookResult struct {
	LimitOrders  []LimitOrder // The channel on which the orders are delivered.
	PoolID       uint64       // The PoolID ID.
	IsBestEffort bool
	Error        error
}
