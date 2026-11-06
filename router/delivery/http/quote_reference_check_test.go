package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/labstack/echo/v4"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/log"
	routerdelivery "github.com/osmosis-labs/sqs/router/delivery/http"
	"github.com/osmosis-labs/sqs/router/usecase"
)

const (
	refUSDC = "uusdc"   // 6 decimals
	refBTC  = "satoshi" // 8 decimals
	refETH  = "wei"     // 18 decimals
	refNone = "unpriced"
)

var refScalingFactors = map[string]osmomath.Dec{
	refUSDC: osmomath.NewDec(1_000_000),
	refBTC:  osmomath.NewDec(100_000_000),
	refETH:  osmomath.MustNewDecFromStr("1000000000000000000"),
	refNone: osmomath.NewDec(1_000_000),
}

// referencePricesMock serves fixed USD prices. refNone has no price.
type referencePricesMock struct{}

func (referencePricesMock) GetPoolDenomMetadata(denom string) (domain.PoolDenomMetaData, error) {
	switch denom {
	case refUSDC:
		return domain.PoolDenomMetaData{Price: osmomath.OneBigDec()}, nil
	case refBTC:
		return domain.PoolDenomMetaData{Price: osmomath.NewBigDec(100_000)}, nil
	case refETH:
		return domain.PoolDenomMetaData{Price: osmomath.NewBigDec(4_000)}, nil
	case refNone:
		return domain.PoolDenomMetaData{Price: osmomath.ZeroBigDec()}, nil
	}
	return domain.PoolDenomMetaData{}, fmt.Errorf("no metadata for %s", denom)
}

func (referencePricesMock) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	scalingFactor, ok := refScalingFactors[denom]
	if !ok {
		return osmomath.Dec{}, fmt.Errorf("no scaling factor for %s", denom)
	}
	return scalingFactor, nil
}

func TestComputeDeviationFromReference(t *testing.T) {
	prices := referencePricesMock{}
	oneEth := osmomath.MustNewDecFromStr("1000000000000000000").TruncateInt()

	tests := []struct {
		name              string
		denomIn, denomOut string
		amountIn          osmomath.Int
		amountOut         osmomath.Int
		expectedOK        bool
		expectedDeviation string
	}{
		{
			// Exact in: 100,000 USDC (6 decimals) in for 0.94 BTC (8 decimals) out, BTC at $100,000.
			name:              "exact in, 6 to 8 decimals, adverse",
			denomIn:           refUSDC,
			denomOut:          refBTC,
			amountIn:          osmomath.NewInt(100_000_000_000),
			amountOut:         osmomath.NewInt(94_000_000),
			expectedOK:        true,
			expectedDeviation: "-0.060000000000000000",
		},
		{
			// Exact out: 1 BTC out costs 125,000 USDC in.
			name:              "exact out, 6 to 8 decimals, adverse",
			denomIn:           refUSDC,
			denomOut:          refBTC,
			amountIn:          osmomath.NewInt(125_000_000_000),
			amountOut:         osmomath.NewInt(100_000_000),
			expectedOK:        true,
			expectedDeviation: "-0.200000000000000000",
		},
		{
			// 1 ETH (18 decimals) in for 3,960 USDC (6 decimals) out, ETH at $4,000.
			name:              "18 to 6 decimals",
			denomIn:           refETH,
			denomOut:          refUSDC,
			amountIn:          oneEth,
			amountOut:         osmomath.NewInt(3_960_000_000),
			expectedOK:        true,
			expectedDeviation: "-0.010000000000000000",
		},
		{
			// 0.01 BTC ($1,000) in for 0.275 ETH ($1,100) out: a favourable quote is positive.
			name:              "8 to 18 decimals, favourable",
			denomIn:           refBTC,
			denomOut:          refETH,
			amountIn:          osmomath.NewInt(1_000_000),
			amountOut:         osmomath.MustNewDecFromStr("275000000000000000").TruncateInt(),
			expectedOK:        true,
			expectedDeviation: "0.100000000000000000",
		},
		{
			name:              "zero out is a deviation of -1",
			denomIn:           refUSDC,
			denomOut:          refBTC,
			amountIn:          osmomath.NewInt(1_000_000),
			amountOut:         osmomath.ZeroInt(),
			expectedOK:        true,
			expectedDeviation: "-1.000000000000000000",
		},
		{
			name:      "nil amount in",
			denomIn:   refUSDC,
			denomOut:  refBTC,
			amountIn:  osmomath.Int{},
			amountOut: osmomath.NewInt(1),
		},
		{
			name:      "zero amount in",
			denomIn:   refUSDC,
			denomOut:  refBTC,
			amountIn:  osmomath.ZeroInt(),
			amountOut: osmomath.NewInt(1),
		},
		{
			name:      "implausibly favourable ratio",
			denomIn:   refUSDC,
			denomOut:  refBTC,
			amountIn:  osmomath.NewInt(1),
			amountOut: osmomath.NewInt(100_000_000_000_000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadataIn, _ := prices.GetPoolDenomMetadata(tt.denomIn)
			metadataOut, _ := prices.GetPoolDenomMetadata(tt.denomOut)

			deviation, ok := routerdelivery.ComputeDeviationFromReference(tt.amountIn, tt.amountOut, metadataIn.Price, metadataOut.Price, refScalingFactors[tt.denomIn], refScalingFactors[tt.denomOut])
			require.Equal(t, tt.expectedOK, ok)
			if tt.expectedOK {
				require.Equal(t, tt.expectedDeviation, deviation.String())
			}
		})
	}
}

func TestQuoteReferenceChecker(t *testing.T) {
	enabled := domain.QuoteReferenceCheckConfig{
		Enabled:       true,
		WarnDeviation: 0.2,
		DustThreshold: 0.99,
	}
	withDust := enabled
	withDust.RefuseDustQuotes = true

	// 100,000 USDC in for BTC out at the given fraction of the reference value.
	usdcIn := osmomath.NewInt(100_000_000_000)
	btcOut := func(fractionOfReference string) osmomath.Int {
		return osmomath.MustNewDecFromStr(fractionOfReference).MulInt64(100_000_000).TruncateInt()
	}

	tests := []struct {
		name               string
		config             domain.QuoteReferenceCheckConfig
		denomOut           string
		amountOut          osmomath.Int
		priceImpact        osmomath.Dec
		priceImpactUnknown bool

		expectedRefused   bool
		expectedDeviation string
		expectedWarning   bool
	}{
		{
			name:              "within warn threshold",
			config:            enabled,
			denomOut:          refBTC,
			amountOut:         btcOut("0.9"),
			expectedDeviation: "-0.100000000000000000",
		},
		{
			name:              "beyond warn threshold",
			config:            enabled,
			denomOut:          refBTC,
			amountOut:         btcOut("0.75"),
			expectedDeviation: "-0.250000000000000000",
			expectedWarning:   true,
		},
		{
			name:              "far beyond warn threshold with refusal disabled",
			config:            enabled,
			denomOut:          refBTC,
			amountOut:         btcOut("0.4"),
			expectedDeviation: "-0.600000000000000000",
			expectedWarning:   true,
		},
		{
			// Would be far beyond any threshold if it were priced.
			name:      "no reference price is skipped",
			config:    enabled,
			denomOut:  refNone,
			amountOut: osmomath.NewInt(1),
		},
		{
			name:               "no reference price and unknown price impact is skipped",
			config:             withDust,
			denomOut:           refNone,
			amountOut:          osmomath.NewInt(1),
			priceImpact:        osmomath.ZeroDec(),
			priceImpactUnknown: true,
		},
		{
			name:            "dust by price impact without a reference price",
			config:          withDust,
			denomOut:        refNone,
			amountOut:       osmomath.NewInt(1),
			priceImpact:     osmomath.MustNewDecFromStr("-0.995"),
			expectedRefused: true,
		},
		{
			name:              "deviation beyond dust threshold only warns",
			config:            withDust,
			denomOut:          refBTC,
			amountOut:         btcOut("0.005"),
			priceImpact:       osmomath.ZeroDec(),
			expectedDeviation: "-0.995000000000000000",
			expectedWarning:   true,
		},
		{
			name:              "dust rule off",
			config:            enabled,
			denomOut:          refBTC,
			amountOut:         btcOut("0.005"),
			priceImpact:       osmomath.MustNewDecFromStr("-0.995"),
			expectedDeviation: "-0.995000000000000000",
			expectedWarning:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker, err := routerdelivery.NewQuoteReferenceChecker(tt.config, referencePricesMock{}, &log.NoOpLogger{})
			require.NoError(t, err)
			require.NotNil(t, checker)

			quote := &usecase.QuoteExactAmountIn{
				PriceImpact:        tt.priceImpact,
				PriceImpactUnknown: tt.priceImpactUnknown,
			}

			err = checker.Check(quote, refUSDC, usdcIn, tt.denomOut, tt.amountOut)

			if tt.expectedRefused {
				var refused domain.QuoteRefusedError
				require.True(t, errors.As(err, &refused))
				require.Nil(t, quote.DeviationFromReference)
				return
			}

			require.NoError(t, err)
			if tt.expectedDeviation == "" {
				require.Nil(t, quote.DeviationFromReference)
				require.False(t, quote.DeviationWarning)
				return
			}

			require.NotNil(t, quote.DeviationFromReference)
			require.Equal(t, tt.expectedDeviation, quote.DeviationFromReference.String())
			require.Equal(t, tt.expectedWarning, quote.DeviationWarning)

			bz, err := json.Marshal(quote)
			require.NoError(t, err)
			require.Contains(t, string(bz), fmt.Sprintf(`"deviation_from_reference":"%s"`, tt.expectedDeviation))
			if tt.expectedWarning {
				require.Contains(t, string(bz), `"deviation_warning":true`)
			} else {
				require.NotContains(t, string(bz), `"deviation_warning"`)
			}
		})
	}
}

// TestQuoteReferenceCheckOffByDefault checks that the default config leaves the check off,
// so no checker is built and the handler leaves quotes untouched, and that unset fields
// are not serialized.
func TestQuoteReferenceCheckOffByDefault(t *testing.T) {
	require.False(t, domain.DefaultConfig.Router.QuoteReferenceCheck.Enabled)

	for _, quote := range []any{&usecase.QuoteExactAmountIn{}, &usecase.QuoteExactAmountOut{}} {
		bz, err := json.Marshal(quote)
		require.NoError(t, err)
		require.NotContains(t, string(bz), "deviation_from_reference")
		require.NotContains(t, string(bz), "deviation_warning")
	}
}

func TestQuoteReferenceCheckConfigThresholds(t *testing.T) {
	_, err := domain.DefaultConfig.Router.QuoteReferenceCheck.Thresholds()
	require.NoError(t, err)

	for _, config := range []domain.QuoteReferenceCheckConfig{
		{WarnDeviation: -0.1},
		{RefuseDustQuotes: true, DustThreshold: 0},
	} {
		_, err := config.Thresholds()
		require.Error(t, err)
	}
}

// TestQuoteReferenceCheckEnabledDefaults checks the defaults once the check is enabled:
// dust quotes are refused at -99% price impact, deviations warn at -20% and never refuse.
func TestQuoteReferenceCheckEnabledDefaults(t *testing.T) {
	config := domain.DefaultConfig.Router.QuoteReferenceCheck
	config.Enabled = true

	checker, err := routerdelivery.NewQuoteReferenceChecker(config, referencePricesMock{}, &log.NoOpLogger{})
	require.NoError(t, err)
	require.NotNil(t, checker)

	usdcIn := osmomath.NewInt(100_000_000_000)

	// -60%: warned, not refused.
	quote := &usecase.QuoteExactAmountIn{PriceImpact: osmomath.ZeroDec()}
	require.NoError(t, checker.Check(quote, refUSDC, usdcIn, refBTC, osmomath.NewInt(40_000_000)))
	require.True(t, quote.DeviationWarning)

	// -99.5% deviation: warned, not refused.
	quote = &usecase.QuoteExactAmountIn{PriceImpact: osmomath.ZeroDec()}
	require.NoError(t, checker.Check(quote, refUSDC, usdcIn, refBTC, osmomath.NewInt(500_000)))
	require.True(t, quote.DeviationWarning)

	// -99.5% price impact: refused as dust.
	quote = &usecase.QuoteExactAmountIn{PriceImpact: osmomath.MustNewDecFromStr("-0.995")}
	err = checker.Check(quote, refUSDC, usdcIn, refBTC, osmomath.NewInt(40_000_000))
	var refused domain.QuoteRefusedError
	require.True(t, errors.As(err, &refused))
	require.Equal(t, "insufficient liquidity: this amount cannot be filled at an acceptable price (price impact -99.50%, limit -99.00%); try a smaller amount", err.Error())
}

func TestQuoteRefusedErrorMessage(t *testing.T) {
	err := domain.QuoteRefusedError{
		PriceImpact: osmomath.MustNewDecFromStr("-0.99999"),
		Threshold:   osmomath.MustNewDecFromStr("0.99"),
	}
	require.Equal(t, "insufficient liquidity: this amount cannot be filled at an acceptable price (price impact -100.00%, limit -99.00%); try a smaller amount", err.Error())
}

// handlerReferencePrices prices the handler test denoms.
type handlerReferencePrices map[string]osmomath.BigDec

func (p handlerReferencePrices) GetPoolDenomMetadata(denom string) (domain.PoolDenomMetaData, error) {
	price, ok := p[denom]
	if !ok {
		return domain.PoolDenomMetaData{}, fmt.Errorf("no metadata for %s", denom)
	}
	return domain.PoolDenomMetaData{Price: price}, nil
}

func (p handlerReferencePrices) GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error) {
	return osmomath.NewDec(1_000_000), nil
}

// TestGetOptimalQuoteReferenceCheck checks the handler wiring: a priced quote carries the
// deviation, and a large deviation warns without refusing.
func (s *RouterHandlerSuite) TestGetOptimalQuoteReferenceCheck() {
	_, poolOne := s.PoolOne()
	_, poolTwo := s.PoolTwo()
	_, poolThree := s.PoolThree()

	const (
		denomIn  = "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5"
		denomOut = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	)

	config := domain.DefaultConfig.Router.QuoteReferenceCheck
	config.Enabled = true

	run := func(prices handlerReferencePrices) *httptest.ResponseRecorder {
		checker, err := routerdelivery.NewQuoteReferenceChecker(config, prices, &log.NoOpLogger{})
		s.Require().NoError(err)

		handler := &routerdelivery.RouterHandler{
			TUsecase: &mocks.TokensUsecaseMock{
				GetPoolDenomsMetadataFunc: func(chainDenoms []string) domain.PoolDenomMetaDataMap {
					return domain.PoolDenomMetaDataMap{}
				},
				IsValidChainDenomFunc: func(chainDenom string) bool {
					return true
				},
			},
			RUsecase: &mocks.RouterUsecaseMock{
				GetOptimalQuoteOutGivenInFunc: func(ctx context.Context, tokenIn sdk.Coin, tokenOutDenom string, opts ...domain.RouterOption) (domain.Quote, error) {
					return s.NewExactAmountInQuote(poolOne, poolTwo, poolThree), nil
				},
			},
			ReferenceCheck: checker,
		}

		e := echo.New()
		req := httptest.NewRequest(echo.GET, "/", nil)
		q := req.URL.Query()
		q.Add("tokenIn", "1000"+denomIn)
		q.Add("tokenOutDenom", denomOut)
		q.Add("singleRoute", "true")
		req.URL.RawQuery = q.Encode()
		rec := httptest.NewRecorder()

		s.Require().NoError(handler.GetOptimalQuote(e.NewContext(req, rec)))
		return rec
	}

	// Equal prices: the quote is served with its deviation.
	rec := run(handlerReferencePrices{denomIn: osmomath.OneBigDec(), denomOut: osmomath.OneBigDec()})
	s.Require().Equal(http.StatusOK, rec.Code)
	s.Require().Contains(rec.Body.String(), `"deviation_from_reference":`)

	// The output is worth almost nothing at the reference prices: served with a warning, since
	// a deviation from the reference never refuses a quote.
	rec = run(handlerReferencePrices{denomIn: osmomath.NewBigDec(1_000_000), denomOut: osmomath.NewBigDecWithPrec(1, 6)})
	s.Require().Equal(http.StatusOK, rec.Code)
	s.Require().Contains(rec.Body.String(), `"deviation_warning":true`)
}

// coingeckoPricesMock serves fixed CoinGecko prices; denoms not in the map have no listing.
type coingeckoPricesMock map[string]osmomath.BigDec

func (m coingeckoPricesMock) GetPrice(_ context.Context, baseDenom string, _ string, _ ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	price, ok := m[baseDenom]
	if !ok {
		return osmomath.BigDec{}, nil, errors.New("no coingecko price")
	}
	return price, nil, nil
}

// TestQuoteReferenceCheckCoingeckoCrossCheck checks that a chain reference price that
// disagrees with the cached CoinGecko price is replaced by it for the warning only: the
// quote reports reference_source coingecko, and a CoinGecko-based deviation never refuses a
// quote as dust. Unlisted denoms and agreeing prices stay on the chain price.
func (suite *RouterHandlerSuite) TestQuoteReferenceCheckCoingeckoCrossCheck() {
	config := domain.QuoteReferenceCheckConfig{Enabled: true, WarnDeviation: 0.2, RefuseDustQuotes: true, DustThreshold: 0.99, CoingeckoMargin: 0.1}

	// 100,000 USDC in; at the chain price of 100,000 USD/BTC, 0.75 BTC out is -25%.
	usdcIn := osmomath.NewInt(100_000_000_000)

	tests := []struct {
		name              string
		coingecko         coingeckoPricesMock
		amountOut         int64
		expectedDeviation string
		expectedWarning   bool
		expectedSource    string
	}{
		{name: "no coingecko listing keeps the chain price", coingecko: coingeckoPricesMock{}, amountOut: 75_000_000, expectedDeviation: "-0.250000000000000000", expectedWarning: true, expectedSource: "pricing_worker"},
		{name: "agreeing coingecko price keeps the chain price", coingecko: coingeckoPricesMock{refBTC: osmomath.NewBigDec(105_000)}, amountOut: 75_000_000, expectedDeviation: "-0.250000000000000000", expectedWarning: true, expectedSource: "pricing_worker"},
		// 0.75 BTC at 150,000 USD is 112,500 USD for 100,000 USD in: +12.5%, so no warning.
		{name: "disagreement is judged against coingecko", coingecko: coingeckoPricesMock{refBTC: osmomath.NewBigDec(150_000)}, amountOut: 75_000_000, expectedDeviation: "0.125000000000000000", expectedSource: "coingecko"},
		// 0.005 BTC at 10,000 USD is 50 USD for 100,000 USD in: -99.95%. Warns, but is not dust.
		{name: "coingecko deviation beyond dust warns without refusing", coingecko: coingeckoPricesMock{refBTC: osmomath.NewBigDec(10_000)}, amountOut: 500_000, expectedDeviation: "-0.999500000000000000", expectedWarning: true, expectedSource: "coingecko"},
		// The same amount against agreeing prices also only warns: only price impact refuses.
		{name: "agreeing prices beyond dust warn without refusing", coingecko: coingeckoPricesMock{refBTC: osmomath.NewBigDec(100_000)}, amountOut: 500_000, expectedDeviation: "-0.995000000000000000", expectedWarning: true, expectedSource: "pricing_worker"},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			checker, err := routerdelivery.NewQuoteReferenceChecker(config, referencePricesMock{}, &log.NoOpLogger{})
			suite.Require().NoError(err)
			checker.WithCoingecko(tt.coingecko)

			quote := &usecase.QuoteExactAmountIn{PriceImpact: osmomath.ZeroDec()}
			err = checker.Check(quote, refUSDC, usdcIn, refBTC, osmomath.NewInt(tt.amountOut))

			suite.Require().NoError(err)
			suite.Require().NotNil(quote.DeviationFromReference)
			suite.Require().Equal(tt.expectedDeviation, quote.DeviationFromReference.String())
			suite.Require().Equal(tt.expectedWarning, quote.DeviationWarning)
			suite.Require().Equal(tt.expectedSource, quote.ReferenceSource)
		})
	}
}
