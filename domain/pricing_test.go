package domain_test

import (
	"testing"

	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/sqs/domain"
	"github.com/stretchr/testify/require"
)

var zeroBigDec = osmomath.ZeroBigDec()

func TestPricesResultGetPriceForDenom(t *testing.T) {
	var (
		baseDenom  = "base"
		quoteDenom = "quote"
		otherDenom = "other"

		validPrice = osmomath.OneBigDec()
	)

	tests := []struct {
		name string

		pricesResult domain.PricesResult
		baseDenom    string
		quoteDenom   string

		expectedPrice osmomath.BigDec
	}{
		{
			name: "empty prices result",

			baseDenom:  baseDenom,
			quoteDenom: quoteDenom,

			pricesResult: domain.PricesResult{},

			expectedPrice: zeroBigDec,
		},
		{
			name: "valid prices result",

			baseDenom:  baseDenom,
			quoteDenom: quoteDenom,

			pricesResult: domain.PricesResult{
				baseDenom: map[string]osmomath.BigDec{
					quoteDenom: validPrice,
				},
			},

			expectedPrice: validPrice,
		},
		{
			name: "other quote",

			baseDenom:  baseDenom,
			quoteDenom: otherDenom,

			pricesResult: domain.PricesResult{
				baseDenom: map[string]osmomath.BigDec{
					quoteDenom: validPrice,
				},
			},

			expectedPrice: zeroBigDec,
		},
		{
			name: "other base",

			baseDenom:  otherDenom,
			quoteDenom: baseDenom,

			pricesResult: domain.PricesResult{
				baseDenom: map[string]osmomath.BigDec{
					quoteDenom: validPrice,
				},
			},

			expectedPrice: zeroBigDec,
		},
	}
	for _, tt := range tests {

		t.Run(tt.name, func(t *testing.T) {

			price := tt.pricesResult.GetPriceForDenom(tt.baseDenom, tt.quoteDenom)

			// Check if the actual output matches the expected output
			require.Equal(t, tt.expectedPrice, price)
		})
	}
}
