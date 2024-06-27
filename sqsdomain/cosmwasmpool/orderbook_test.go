package cosmwasmpool_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/sqsdomain/cosmwasmpool"
	"github.com/stretchr/testify/assert"
)

const (
	QUOTE_DENOM   = "quote"
	BASE_DENOM    = "base"
	INVALID_DENOM = "invalid"
)

func TestGetDirection(t *testing.T) {
	tests := map[string]struct {
		tokenInDenom  string
		tokenOutDenom string
		expected      cosmwasmpool.OrderbookDirection
		expectError   error
	}{
		"BID direction": {
			tokenInDenom:  QUOTE_DENOM,
			tokenOutDenom: BASE_DENOM,
			expected:      cosmwasmpool.BID,
		},
		"ASK direction": {
			tokenInDenom:  BASE_DENOM,
			tokenOutDenom: QUOTE_DENOM,
			expected:      cosmwasmpool.ASK,
		},
		"duplicated denom": {
			tokenInDenom:  BASE_DENOM,
			tokenOutDenom: BASE_DENOM,
			expectError: cosmwasmpool.DuplicatedDenomError{
				Denom: BASE_DENOM,
			},
		},
		"invalid direction": {
			tokenInDenom:  INVALID_DENOM,
			tokenOutDenom: BASE_DENOM,
			expectError: cosmwasmpool.OrderbookUnsupportedDenomError{
				Denom:      INVALID_DENOM,
				BaseDenom:  BASE_DENOM,
				QuoteDenom: QUOTE_DENOM,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)

			orderbookData := cosmwasmpool.OrderbookData{}

			direction, err := orderbookData.GetDirection(tc.tokenInDenom, tc.tokenOutDenom)

			if tc.expectError != nil {
				assert.Error(err)
				assert.Equal(err, tc.expectError)
				return
			}
			assert.NoError(err)
			assert.Equal(tc.expected, *direction)
		})
	}
}

func TestGetFillableAmount(t *testing.T) {
	tests := map[string]struct {
		input        osmomath.BigDec
		direction    cosmwasmpool.OrderbookDirection
		bidLiquidity osmomath.BigDec
		askLiquidity osmomath.BigDec
		expected     osmomath.BigDec
	}{
		"fillable amount less than tick liquidity": {
			input:        osmomath.NewBigDec(50),
			direction:    cosmwasmpool.BID,
			bidLiquidity: osmomath.NewBigDec(100),
			askLiquidity: osmomath.NewBigDec(0),
			expected:     osmomath.NewBigDec(50),
		},
		"fillable amount more than tick liquidity": {
			input:        osmomath.NewBigDec(150),
			direction:    cosmwasmpool.ASK,
			bidLiquidity: osmomath.NewBigDec(0),
			askLiquidity: osmomath.NewBigDec(100),
			expected:     osmomath.NewBigDec(100),
		},
		"fillable amount equal to tick liquidity": {
			input:        osmomath.NewBigDec(100),
			direction:    cosmwasmpool.BID,
			bidLiquidity: osmomath.NewBigDec(100),
			askLiquidity: osmomath.NewBigDec(0),
			expected:     osmomath.NewBigDec(100),
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert := assert.New(t)

			orderbookTickLiquidity := cosmwasmpool.OrderbookTickLiquidity{
				BidLiquidity: tc.bidLiquidity,
				AskLiquidity: tc.askLiquidity,
			}

			fillableAmount := orderbookTickLiquidity.GetFillableAmount(tc.input, tc.direction)

			assert.Equal(tc.expected, fillableAmount)
		})
	}
}
