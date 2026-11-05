package domain

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// QuoteReferenceCheckConfig configures the sanity check of user quotes against a reference
// price. The reference is the USD value of both sides of the quote at the chain pricer's
// cached prices, independent of the route the router chose.
//
// The deviation is valueOut / valueIn - 1, so it is negative when the quote is worse than
// the reference. The thresholds below are magnitudes of an adverse deviation, e.g. 0.2
// means 20% worse than the reference.
type QuoteReferenceCheckConfig struct {
	// Enabled turns the check on. When off, quote responses and behaviour are unchanged.
	Enabled bool `mapstructure:"enabled"`

	// WarnDeviation is the adverse deviation at or beyond which the quote is returned
	// with a warning. Zero disables the warning.
	WarnDeviation float64 `mapstructure:"warn-deviation"`

	// RefuseDustQuotes refuses quotes whose own price impact is at or beyond DustThreshold,
	// i.e. quotes that return close to nothing. The deviation from the reference price
	// never refuses a quote, only warns. On by default, but only applies when Enabled is set.
	RefuseDustQuotes bool `mapstructure:"refuse-dust-quotes"`

	// DustThreshold is the adverse price impact that makes a quote dust when
	// RefuseDustQuotes is set, e.g. 0.99 for -99%.
	DustThreshold float64 `mapstructure:"dust-threshold"`

	// CoingeckoMargin is the relative difference between a denom's chain reference price
	// and the CoinGecko price SQS already caches (as |chain / coingecko - 1|) beyond which
	// the chain price is not trusted and the CoinGecko price is the reference for the warning
	// (never for a refusal; see QuoteReferenceChecker.WithCoingecko). Only denoms with a fresh CoinGecko price are
	// compared; others stay on the chain price. Zero disables the cross-check.
	CoingeckoMargin float64 `mapstructure:"coingecko-margin"`
}

// QuoteReferenceThresholds holds the parsed thresholds of QuoteReferenceCheckConfig.
// Each value is a non-negative magnitude; zero means the rule is disabled.
type QuoteReferenceThresholds struct {
	Warn osmomath.Dec
	Dust osmomath.Dec
	// CoingeckoMargin is zero when the cross-check is disabled.
	CoingeckoMargin osmomath.Dec
}

// Thresholds parses and validates the configured thresholds.
// Returns an error if any threshold is negative, above one or not representable
// as a decimal, or if dust refusals are enabled without a positive dust threshold.
func (c QuoteReferenceCheckConfig) Thresholds() (QuoteReferenceThresholds, error) {
	warn, err := parseQuoteReferenceThreshold("warn-deviation", c.WarnDeviation)
	if err != nil {
		return QuoteReferenceThresholds{}, err
	}

	dust, err := parseQuoteReferenceThreshold("dust-threshold", c.DustThreshold)
	if err != nil {
		return QuoteReferenceThresholds{}, err
	}

	margin, err := parseQuoteReferenceThreshold("coingecko-margin", c.CoingeckoMargin)
	if err != nil {
		return QuoteReferenceThresholds{}, err
	}

	if c.RefuseDustQuotes && !dust.IsPositive() {
		return QuoteReferenceThresholds{}, fmt.Errorf("quote-reference-check: dust-threshold must be positive when refuse-dust-quotes is set")
	}

	return QuoteReferenceThresholds{
		Warn: warn,
		Dust: dust,

		CoingeckoMargin: margin,
	}, nil
}

func parseQuoteReferenceThreshold(name string, value float64) (osmomath.Dec, error) {
	if value < 0 || value > 1 {
		return osmomath.Dec{}, fmt.Errorf("quote-reference-check: %s must be between 0 and 1, got %v", name, value)
	}

	dec, err := osmomath.NewDecFromStr(strconv.FormatFloat(value, 'f', -1, 64))
	if err != nil {
		return osmomath.Dec{}, fmt.Errorf("quote-reference-check: invalid %s (%v): %w", name, value, err)
	}

	return dec, nil
}

// Values of the quote's reference_source field: the prices the reported deviation is based on.
// The pricing worker's cached price is mostly derived from pools, but it can itself be a CoinGecko
// fallback for a token whose pools are below the worker's liquidity floor, so it is not called "chain".
const (
	QuoteReferenceSourcePricingWorker = "pricing_worker"
	QuoteReferenceSourceCoingecko     = "coingecko"
)

// QuoteRefusedError is returned instead of a quote when the quote's own price impact is at
// or beyond the dust threshold. It signals that the pools SQS routes through cannot fill the
// amount at an acceptable price, rather than that no route exists.
type QuoteRefusedError struct {
	// PriceImpact is the quote's price impact that triggered the refusal (negative).
	PriceImpact osmomath.Dec
	// Threshold is the configured magnitude that PriceImpact reached.
	Threshold osmomath.Dec
}

// Error returns a message that reads on its own, since clients may show it as is.
func (e QuoteRefusedError) Error() string {
	return fmt.Sprintf("insufficient liquidity: this amount cannot be filled at an acceptable price (price impact %s, limit -%s); try a smaller amount",
		formatPercent(e.PriceImpact), formatPercent(e.Threshold))
}

// formatPercent formats a decimal fraction as a percentage with two decimals, e.g.
// -0.995 as "-99.50%". It is for display only.
func formatPercent(d osmomath.Dec) string {
	if d.IsNil() {
		return "unknown"
	}

	percent := d.MulInt64(100)
	sign := ""
	if percent.IsNegative() {
		sign = "-"
		percent = percent.Neg()
	}

	// Round half up to two decimals, then split into integer and fractional parts.
	hundredths := percent.MulInt64(100).RoundInt()
	whole := hundredths.QuoRaw(100)
	fraction := hundredths.ModRaw(100).String()
	if len(fraction) < 2 {
		fraction = strings.Repeat("0", 2-len(fraction)) + fraction
	}

	return fmt.Sprintf("%s%s.%s%%", sign, whole, fraction)
}
