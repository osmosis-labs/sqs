package orderbookdomain_test

import (
	"testing"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

	"github.com/osmosis-labs/osmosis/osmomath"

	"github.com/stretchr/testify/assert"
)

func TestOrderStatus(t *testing.T) {
	tests := []struct {
		name          string
		order         orderbookdomain.Order
		percentFilled float64
		expected      orderbookdomain.OrderStatus
		expectError   bool
	}{
		{
			name:          "Valid quantity, percentFilled = 0",
			order:         orderbookdomain.Order{Quantity: "100.0"},
			percentFilled: 0,
			expected:      orderbookdomain.StatusOpen,
			expectError:   false,
		},
		{
			name:          "Valid quantity, percentFilled = 1",
			order:         orderbookdomain.Order{Quantity: "100.0"},
			percentFilled: 1,
			expected:      orderbookdomain.StatusFilled,
			expectError:   false,
		},
		{
			name:          "Valid quantity, percentFilled < 1",
			order:         orderbookdomain.Order{Quantity: "100.0"},
			percentFilled: 0.5,
			expected:      orderbookdomain.StatusPartiallyFilled,
			expectError:   false,
		},
		{
			name:          "Zero quantity",
			order:         orderbookdomain.Order{Quantity: "0"},
			percentFilled: 1,
			expected:      orderbookdomain.StatusFilled,
			expectError:   false,
		},
		{
			name:          "Invalid quantity string",
			order:         orderbookdomain.Order{Quantity: "invalid"},
			percentFilled: 1,
			expectError:   true,
		},
		{
			name:          "Empty quantity string",
			order:         orderbookdomain.Order{Quantity: ""},
			percentFilled: 1,
			expectError:   true,
		},
		{
			name:          "Out of range quantity string",
			order:         orderbookdomain.Order{Quantity: "101960000000000000000"},
			expected:      orderbookdomain.StatusFilled,
			percentFilled: 1,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := tt.order.Status(tt.percentFilled)
			if tt.expectError {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expected, status)
		})
	}
}

func TestOrdersByDirection(t *testing.T) {
	testCases := []struct {
		name           string
		orders         orderbookdomain.Orders
		direction      string
		expectedOrders orderbookdomain.Orders
	}{
		{
			name: "Filter buy orders",
			orders: orderbookdomain.Orders{
				{OrderDirection: "buy", OrderId: 1},
				{OrderDirection: "sell", OrderId: 2},
				{OrderDirection: "buy", OrderId: 3},
			},
			direction: "buy",
			expectedOrders: orderbookdomain.Orders{
				{OrderDirection: "buy", OrderId: 1},
				{OrderDirection: "buy", OrderId: 3},
			},
		},
		{
			name: "Filter sell orders",
			orders: orderbookdomain.Orders{
				{OrderDirection: "buy", OrderId: 1},
				{OrderDirection: "sell", OrderId: 2},
				{OrderDirection: "buy", OrderId: 3},
				{OrderDirection: "sell", OrderId: 4},
			},
			direction: "sell",
			expectedOrders: orderbookdomain.Orders{
				{OrderDirection: "sell", OrderId: 2},
				{OrderDirection: "sell", OrderId: 4},
			},
		},
		{
			name: "No matching orders",
			orders: orderbookdomain.Orders{
				{OrderDirection: "buy", OrderId: 1},
				{OrderDirection: "buy", OrderId: 2},
			},
			direction:      "sell",
			expectedOrders: nil,
		},
		{
			name:           "Empty orders slice",
			orders:         orderbookdomain.Orders{},
			direction:      "buy",
			expectedOrders: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.orders.OrderByDirection(tc.direction)
			assert.Equal(t, tc.expectedOrders, result)
		})
	}
}

func TestLimitOrder_IsClaimable(t *testing.T) {
	tests := []struct {
		name      string
		order     orderbookdomain.LimitOrder
		threshold osmomath.Dec
		want      bool
	}{
		{
			name: "Fully filled order",
			order: orderbookdomain.LimitOrder{
				PercentFilled: osmomath.NewDec(1),
			},
			threshold: osmomath.NewDecWithPrec(4, 1), // 0.4
			want:      true,
		},
		{
			name: "Partially filled order above threshold",
			order: orderbookdomain.LimitOrder{
				PercentFilled: osmomath.NewDecWithPrec(75, 2), // 0.75
			},
			threshold: osmomath.NewDecWithPrec(6, 1), // 0.6
			want:      true,
		},
		{
			name: "Partially filled order below threshold",
			order: orderbookdomain.LimitOrder{
				PercentFilled: osmomath.NewDecWithPrec(85, 2), // 0.85
			},
			threshold: osmomath.NewDecWithPrec(9, 1), // 0.9
			want:      false,
		},
		{
			name: "Unfilled order",
			order: orderbookdomain.LimitOrder{
				PercentFilled: osmomath.NewDec(0),
			},
			threshold: osmomath.NewDecWithPrec(1, 1), // 0.1
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.order.IsClaimable(tt.threshold)
			assert.Equal(t, tt.want, got)
		})
	}
}
