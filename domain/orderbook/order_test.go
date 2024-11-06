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
		direction      orderbookdomain.OrderDirection
		expectedOrders orderbookdomain.Orders
	}{
		{
			name: "Filter buy orders",
			orders: orderbookdomain.Orders{
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 1},
				{OrderDirection: orderbookdomain.DirectionAsk.String(), OrderId: 2},
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 3},
			},
			direction: orderbookdomain.DirectionBid,
			expectedOrders: orderbookdomain.Orders{
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 1},
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 3},
			},
		},
		{
			name: "Filter sell orders",
			orders: orderbookdomain.Orders{
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 1},
				{OrderDirection: orderbookdomain.DirectionAsk.String(), OrderId: 2},
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 3},
				{OrderDirection: orderbookdomain.DirectionAsk.String(), OrderId: 4},
			},
			direction: orderbookdomain.DirectionAsk,
			expectedOrders: orderbookdomain.Orders{
				{OrderDirection: orderbookdomain.DirectionAsk.String(), OrderId: 2},
				{OrderDirection: orderbookdomain.DirectionAsk.String(), OrderId: 4},
			},
		},
		{
			name: "No matching orders",
			orders: orderbookdomain.Orders{
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 1},
				{OrderDirection: orderbookdomain.DirectionBid.String(), OrderId: 2},
			},
			direction:      orderbookdomain.DirectionAsk,
			expectedOrders: nil,
		},
		{
			name:           "Empty orders slice",
			orders:         orderbookdomain.Orders{},
			direction:      orderbookdomain.DirectionBid,
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

func TestClaimableAmount(t *testing.T) {
	tests := []struct {
		name  string
		order orderbookdomain.LimitOrder
		want  osmomath.Dec
	}{
		{
			name: "Buy 10 OSMO for 1 USD with 50% filled and 0.25% claimed",
			order: orderbookdomain.LimitOrder{
				OrderDirection: orderbookdomain.DirectionBid,
				TotalFilled:    osmomath.NewDec(5),
				PercentClaimed: osmomath.MustNewDecFromStr("0.25"),
			},
			want: osmomath.MustNewDecFromStr("3.75"),
		},
		{
			name: "Buy 10 OSMO with 0% filled and 1% claimed",
			order: orderbookdomain.LimitOrder{
				OrderDirection: orderbookdomain.DirectionBid,
				TotalFilled:    osmomath.NewDec(0),
				PercentClaimed: osmomath.MustNewDecFromStr("1"),
			},
			want: osmomath.MustNewDecFromStr("0"),
		},
		{
			name: "Buy 10 OSMO with 5% filled and 0% claimed",
			order: orderbookdomain.LimitOrder{
				OrderDirection: orderbookdomain.DirectionBid,
				TotalFilled:    osmomath.NewDec(5),
				PercentClaimed: osmomath.MustNewDecFromStr("0"),
			},
			want: osmomath.MustNewDecFromStr("5"),
		},
		{
			name: "Sell 0.1 OSMO for 1 USD, with 50% filled and 0.25% claimed",
			order: orderbookdomain.LimitOrder{
				OrderDirection: orderbookdomain.DirectionAsk,
				Output:         osmomath.NewDec(10),
				TotalFilled:    osmomath.MustNewDecFromStr("0.5"),
				PercentClaimed: osmomath.MustNewDecFromStr("0.25"),
			},
			want: osmomath.MustNewDecFromStr("0.375"),
		},
		{
			name: "Sell 0.1 OSMO for 1 USD, with 0% filled and 2% claimed",
			order: orderbookdomain.LimitOrder{
				OrderDirection: orderbookdomain.DirectionAsk,
				Output:         osmomath.NewDec(10),
				TotalFilled:    osmomath.NewDec(0),
				PercentClaimed: osmomath.MustNewDecFromStr("0.2"),
			},
			want: osmomath.NewDec(0),
		},
		{
			name: "Sell 0.1 OSMO for 1 USD, with 3% filled and 0% claimed",
			order: orderbookdomain.LimitOrder{
				OrderDirection: orderbookdomain.DirectionAsk,
				Output:         osmomath.NewDec(10),
				TotalFilled:    osmomath.MustNewDecFromStr("0.3"),
				PercentClaimed: osmomath.MustNewDecFromStr("0"),
			},
			want: osmomath.MustNewDecFromStr("0.3"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.order.ClaimableAmount()
			assert.Equal(t, tt.want, got)
		})
	}
}
