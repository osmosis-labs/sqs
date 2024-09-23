package orderbookdomain_test

import (
	"testing"

	orderbookdomain "github.com/osmosis-labs/sqs/domain/orderbook"

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
