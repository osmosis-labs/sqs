package domain

import (
	"context"
	"fmt"
	"strconv"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// QuoteUSDPegConfig configures the check of the pricing quote denom (allUSDC) against US dollars.
// SQS prices every asset in that denom and treats it as one dollar, so a depeg of the denom itself
// is invisible to every onchain price. The check compares it with the cached CoinGecko USD price.
type QuoteUSDPegConfig struct {
	// Enabled turns the check on. When off, no response carries the peg.
	Enabled bool `mapstructure:"enabled"`

	// WarnDeviation is the absolute deviation from one dollar at or beyond which the status is warn.
	WarnDeviation float64 `mapstructure:"warn-deviation"`

	// SevereDeviation is the absolute deviation from one dollar at or beyond which the status is severe.
	SevereDeviation float64 `mapstructure:"severe-deviation"`
}

// Values of QuoteUSDPeg.Status.
const (
	QuoteUSDPegStatusOK      = "ok"
	QuoteUSDPegStatusWarn    = "warn"
	QuoteUSDPegStatusSevere  = "severe"
	QuoteUSDPegStatusUnknown = "unknown"
)

// QuoteUSDPeg is the peg of the quote denom against US dollars, as reported on /tokens/prices and
// /router/quote responses. Status unknown means no fresh reference price was available; then Price and
// Deviation are absent. It never stands for a one dollar price or for a depeg.
type QuoteUSDPeg struct {
	Status string `json:"status"`
	// Price is the reference USD price of one unit of the quote denom.
	Price *osmomath.Dec `json:"price,omitempty"`
	// Deviation is price - 1, negative when the quote denom trades below one dollar.
	Deviation *osmomath.Dec `json:"deviation,omitempty"`
}

// USDReferencePriceSource serves cached USD prices, such as the CoinGecko snapshot poller.
// GetPrice never waits on the network and errors for a missing or stale price.
type USDReferencePriceSource interface {
	GetPrice(ctx context.Context, baseDenom string, quoteDenom string, opts ...PricingOption) (osmomath.BigDec, []SplitRoute, error)
}

// QuoteUSDPegChecker evaluates the peg of one denom from a cached USD price.
type QuoteUSDPegChecker struct {
	denom  string
	source USDReferencePriceSource
	warn   osmomath.Dec
	severe osmomath.Dec
}

// Validate checks that 0 < warn <= severe <= 1. It is a no-op when the check is disabled.
func (c QuoteUSDPegConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.WarnDeviation <= 0 || c.SevereDeviation < c.WarnDeviation || c.SevereDeviation > 1 {
		return fmt.Errorf("quote-usd-peg: need 0 < warn-deviation <= severe-deviation <= 1, got %v and %v", c.WarnDeviation, c.SevereDeviation)
	}
	return nil
}

// NewQuoteUSDPegChecker returns a checker of denom (the chain denom prices are quoted in).
func NewQuoteUSDPegChecker(config QuoteUSDPegConfig, denom string, source USDReferencePriceSource) (*QuoteUSDPegChecker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	warn, err := osmomath.NewDecFromStr(strconv.FormatFloat(config.WarnDeviation, 'f', -1, 64))
	if err != nil {
		return nil, err
	}
	severe, err := osmomath.NewDecFromStr(strconv.FormatFloat(config.SevereDeviation, 'f', -1, 64))
	if err != nil {
		return nil, err
	}
	return &QuoteUSDPegChecker{denom: denom, source: source, warn: warn, severe: severe}, nil
}

// Check returns the current peg from the cached reference price: no network call is made. A missing,
// stale or non-positive price yields status unknown.
func (c *QuoteUSDPegChecker) Check(ctx context.Context) *QuoteUSDPeg {
	if c == nil {
		return nil
	}

	price, _, err := c.source.GetPrice(ctx, c.denom, "")
	if err != nil || price.IsNil() || !price.IsPositive() {
		return &QuoteUSDPeg{Status: QuoteUSDPegStatusUnknown}
	}

	p := price.Dec()
	deviation := p.Sub(osmomath.OneDec())

	status := QuoteUSDPegStatusOK
	switch abs := deviation.Abs(); {
	case abs.GTE(c.severe):
		status = QuoteUSDPegStatusSevere
	case abs.GTE(c.warn):
		status = QuoteUSDPegStatusWarn
	}

	return &QuoteUSDPeg{Status: status, Price: &p, Deviation: &deviation}
}
