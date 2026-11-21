package http

import (
	"context"

	"github.com/osmosis-labs/osmosis/osmomath"
	"go.uber.org/zap"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/log"
)

// ReferencePriceSource provides the cached reference prices and chain scaling factors
// used by QuoteReferenceChecker. Both lookups are in-memory reads of data maintained by
// the pricing workers, so the check never triggers a price computation.
type ReferencePriceSource interface {
	GetPoolDenomMetadata(chainDenom string) (domain.PoolDenomMetaData, error)
	GetChainScalingFactorByDenomMut(denom string) (osmomath.Dec, error)
}

// CoingeckoPriceSource serves the CoinGecko USD prices SQS already caches (the snapshot
// poller). GetPrice never waits on the network. It returns an error for a denom without a
// CoinGecko listing and for a price that is missing or older than the poller's max age, both
// of which the check treats as no CoinGecko price.
type CoingeckoPriceSource interface {
	GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error)
}

// Outcomes recorded by the quote reference check metric.
const (
	quoteReferenceOutcomeOK             = "ok"
	quoteReferenceOutcomeWarned         = "warned"
	quoteReferenceOutcomeRefusedDust    = "refused_dust"
	quoteReferenceOutcomeSkippedNoPrice = "skipped_no_price"
)

// maxReferenceRatio bounds the value out / value in ratio that is treated as meaningful.
// A quote this many times better than the reference means the reference price is wrong,
// so the deviation is not reported.
var maxReferenceRatio = osmomath.NewBigDec(1_000_000_000)

// QuoteReferenceChecker sanity-checks user quotes against a reference price that does not
// depend on the route the router chose: the cached USD prices of the token in and the
// token out. It reports the deviation on the quote, flags it beyond the warning threshold,
// and refuses it beyond the refusal or dust thresholds.
type QuoteReferenceChecker struct {
	prices ReferencePriceSource
	logger log.Logger

	// Negated thresholds, so that they compare directly with an adverse (negative) deviation.
	// Zero disables the rule.
	negWarn osmomath.Dec
	negDust osmomath.Dec

	thresholds domain.QuoteReferenceThresholds
	refuseDust bool

	// coingecko is optional; nil disables the cross-check.
	coingecko CoingeckoPriceSource
}

// WithCoingecko enables the per-denom CoinGecko cross-check using the cached prices of source.
// It has no effect when the configured margin is zero. Call it before serving quotes.
//
// Where a denom's chain price and its CoinGecko price differ by more than the margin, the CoinGecko
// price is the reference for the warning only. It never decides a refusal: CoinGecko ids are shared
// between variants of one asset, so a dead variant can carry the canonical coin's price.
func (c *QuoteReferenceChecker) WithCoingecko(source CoingeckoPriceSource) *QuoteReferenceChecker {
	c.coingecko = source
	return c
}

// NewQuoteReferenceChecker returns a checker for the given config. Callers build one only
// when config.Enabled is set. Returns an error if the config thresholds are invalid.
func NewQuoteReferenceChecker(config domain.QuoteReferenceCheckConfig, prices ReferencePriceSource, logger log.Logger) (*QuoteReferenceChecker, error) {
	thresholds, err := config.Thresholds()
	if err != nil {
		return nil, err
	}

	return &QuoteReferenceChecker{
		prices:     prices,
		logger:     logger,
		negWarn:    thresholds.Warn.Neg(),
		negDust:    thresholds.Dust.Neg(),
		thresholds: thresholds,
		refuseDust: config.RefuseDustQuotes,
	}, nil
}

// Check compares the quote with the reference price. amountIn and amountOut are the
// quote's total input and output in chain units of tokenInDenom and tokenOutDenom, for
// either swap method. A nil amount skips the deviation (but not the dust check on price
// impact).
//
// It must be called after PrepareResult, which computes the price impact.
// Returns domain.QuoteRefusedError if the quote must not be served; the handler maps it
// to HTTP 422. Otherwise, sets the deviation on the quote when both reference prices are
// known, and returns nil.
func (c *QuoteReferenceChecker) Check(quote domain.Quote, tokenInDenom string, amountIn osmomath.Int, tokenOutDenom string, amountOut osmomath.Int) error {
	deviation, fromCoingecko, hasDeviation := c.deviationFromReference(tokenInDenom, amountIn, tokenOutDenom, amountOut)

	// Only the quote's own price impact can refuse it as dust. The reference prices may be a
	// CoinGecko fallback or a shared CoinGecko id, so a deviation from them only warns. An
	// unknown price impact is reported as zero, so it never makes a quote dust.
	if c.refuseDust {
		if priceImpact := quote.GetPriceImpact(); !priceImpact.IsNil() && priceImpact.LTE(c.negDust) {
			c.record(quoteReferenceOutcomeRefusedDust, tokenInDenom, amountIn, tokenOutDenom, amountOut, priceImpact)
			return domain.QuoteRefusedError{PriceImpact: priceImpact, Threshold: c.thresholds.Dust}
		}
	}

	if !hasDeviation {
		c.record(quoteReferenceOutcomeSkippedNoPrice, tokenInDenom, amountIn, tokenOutDenom, amountOut, osmomath.Dec{})
		return nil
	}

	warning := c.thresholds.Warn.IsPositive() && deviation.LTE(c.negWarn)
	source := domain.QuoteReferenceSourcePricingWorker
	if fromCoingecko {
		source = domain.QuoteReferenceSourceCoingecko
	}
	quote.SetDeviationFromReference(deviation, warning, source)

	if warning {
		c.record(quoteReferenceOutcomeWarned, tokenInDenom, amountIn, tokenOutDenom, amountOut, deviation)
	} else {
		c.record(quoteReferenceOutcomeOK, tokenInDenom, amountIn, tokenOutDenom, amountOut, deviation)
	}

	return nil
}

// deviationFromReference returns the deviation of the quote from the reference price, whether
// either denom's reference was the CoinGecko price, and true; or false if a reference price,
// scaling factor or amount is missing.
func (c *QuoteReferenceChecker) deviationFromReference(tokenInDenom string, amountIn osmomath.Int, tokenOutDenom string, amountOut osmomath.Int) (osmomath.Dec, bool, bool) {
	priceIn, scalingFactorIn, cgIn, ok := c.referencePrice(tokenInDenom)
	if !ok {
		return osmomath.Dec{}, false, false
	}

	priceOut, scalingFactorOut, cgOut, ok := c.referencePrice(tokenOutDenom)
	if !ok {
		return osmomath.Dec{}, false, false
	}

	deviation, ok := computeDeviationFromReference(amountIn, amountOut, priceIn, priceOut, scalingFactorIn, scalingFactorOut)
	return deviation, cgIn || cgOut, ok
}

// referencePrice returns the cached USD price of a denom with its chain scaling factor.
// The third result is true when the price is the CoinGecko price, used because the chain price
// disagrees with it beyond the margin. Returns false if the chain price or scaling factor is
// missing or not positive: an unpriced token is not checked.
func (c *QuoteReferenceChecker) referencePrice(denom string) (osmomath.BigDec, osmomath.Dec, bool, bool) {
	metadata, err := c.prices.GetPoolDenomMetadata(denom)
	if err != nil || metadata.Price.IsNil() || !metadata.Price.IsPositive() {
		return osmomath.BigDec{}, osmomath.Dec{}, false, false
	}

	scalingFactor, err := c.prices.GetChainScalingFactorByDenomMut(denom)
	if err != nil || scalingFactor.IsNil() || !scalingFactor.IsPositive() {
		return osmomath.BigDec{}, osmomath.Dec{}, false, false
	}

	price := metadata.Price

	// Cross-check the chain price with the cached CoinGecko price. A broken pool can skew the
	// chain price in the same direction as the quote, so a chain price that disagrees with
	// CoinGecko is replaced by it, for the warning only. Denoms without a fresh CoinGecko price
	// stay on the chain price.
	if cgPrice, ok := c.coingeckoPrice(denom); ok {
		diff := price.Quo(cgPrice).Sub(osmomath.OneBigDec()).Abs()
		if diff.GT(osmomath.BigDecFromDec(c.thresholds.CoingeckoMargin)) {
			domain.SQSQuoteReferenceCoingeckoDisagreementCounter.Inc()
			return cgPrice, scalingFactor, true, true
		}
	}

	return price, scalingFactor, false, true
}

// coingeckoPrice returns the cached CoinGecko USD price of a denom, or false if the cross-check
// is off, the denom has no listing, or the price is missing, stale or not positive.
func (c *QuoteReferenceChecker) coingeckoPrice(denom string) (osmomath.BigDec, bool) {
	if c.coingecko == nil || !c.thresholds.CoingeckoMargin.IsPositive() {
		return osmomath.BigDec{}, false
	}

	price, _, err := c.coingecko.GetPrice(context.Background(), denom, "")
	if err != nil || price.IsNil() || !price.IsPositive() {
		return osmomath.BigDec{}, false
	}

	return price, true
}

// computeDeviationFromReference returns valueOut / valueIn - 1, where each value is the
// chain amount divided by its scaling factor (10^exponent) and multiplied by its USD price.
// The result is negative when the quote returns less value than it takes. The same formula
// holds for both swap methods: exact-in fixes amountIn and exact-out fixes amountOut.
//
// Returns false if an amount is missing, amountIn is not positive, or the ratio is beyond
// maxReferenceRatio.
func computeDeviationFromReference(amountIn, amountOut osmomath.Int, priceIn, priceOut osmomath.BigDec, scalingFactorIn, scalingFactorOut osmomath.Dec) (osmomath.Dec, bool) {
	if amountIn.IsNil() || amountOut.IsNil() || !amountIn.IsPositive() || amountOut.IsNegative() {
		return osmomath.Dec{}, false
	}

	// (amountOut * priceOut / scalingFactorOut) / (amountIn * priceIn / scalingFactorIn),
	// rearranged to divide once.
	valueOut := osmomath.BigDecFromSDKInt(amountOut).MulMut(priceOut).MulMut(osmomath.BigDecFromDec(scalingFactorIn))
	valueIn := osmomath.BigDecFromSDKInt(amountIn).MulMut(priceIn).MulMut(osmomath.BigDecFromDec(scalingFactorOut))
	if !valueIn.IsPositive() {
		return osmomath.Dec{}, false
	}

	ratio := valueOut.QuoMut(valueIn)
	if ratio.GT(maxReferenceRatio) {
		return osmomath.Dec{}, false
	}

	return ratio.Dec().SubMut(oneDec), true
}

// record increments the outcome metric and logs flagged quotes. The pair is logged rather
// than used as a metric label to keep label cardinality low.
func (c *QuoteReferenceChecker) record(outcome, tokenInDenom string, amountIn osmomath.Int, tokenOutDenom string, amountOut osmomath.Int, value osmomath.Dec) {
	domain.SQSQuoteReferenceCheckCounter.WithLabelValues(outcome).Inc()

	if outcome == quoteReferenceOutcomeOK {
		return
	}

	fields := []zap.Field{
		zap.String("outcome", outcome),
		zap.String("token_in_denom", tokenInDenom),
		zap.Stringer("amount_in", amountIn),
		zap.String("token_out_denom", tokenOutDenom),
		zap.Stringer("amount_out", amountOut),
	}
	if !value.IsNil() {
		fields = append(fields, zap.Stringer("value", value))
	}

	if outcome == quoteReferenceOutcomeSkippedNoPrice {
		c.logger.Debug("quote reference check skipped", fields...)
		return
	}

	c.logger.Info("quote flagged by reference check", fields...)
}
