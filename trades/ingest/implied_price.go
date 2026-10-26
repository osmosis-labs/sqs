package ingest

import (
	"time"

	"github.com/osmosis-labs/osmosis/osmomath"
)

// impliedPriceWindow is how far back the trades that imply a denom's price
// reach. Against the live spot prices of 49,000 mainnet OSMO trades, a 15
// minute window valued them within 0.3% (median) and 1.5% (p99); 1 and 4 hour
// windows lagged the market further.
const impliedPriceWindow = 15 * time.Minute

// impliedPrices are USD prices per minimal unit implied by the trades read
// in the preceding window, used to value backfilled trades at the time they
// happened. A denom's price is the volume-weighted average over its window:
// the USD value of the trades it was in over the amount of it they moved.
// Observations must be added in time order.
type impliedPrices struct {
	window time.Duration
	denoms map[string]*impliedPrice
}

type impliedPrice struct {
	// observations are ascending by time, and usd and amount are their sums.
	observations []priceObservation
	usd          osmomath.BigDec
	amount       osmomath.BigDec
}

type priceObservation struct {
	at     time.Time
	usd    osmomath.BigDec
	amount osmomath.BigDec
}

// tradeTimePrices are the prices implied by backfilled trades, in two tiers.
// Direct prices are observed from trades with a leg in the USD quote denom,
// derived prices from trades valued at a direct price. A trade valued at a
// derived price is not observed, so every price is at most two trades from the
// USD quote denom and none can feed back into the price it was derived from:
// otherwise a round trip through another asset would re-anchor a price on
// itself, compounding fees and slippage into an apparent price move.
type tradeTimePrices struct {
	direct  *impliedPrices
	derived *impliedPrices
}

func newTradeTimePrices(window time.Duration) *tradeTimePrices {
	return &tradeTimePrices{direct: newImpliedPrices(window), derived: newImpliedPrices(window)}
}

func newImpliedPrices(window time.Duration) *impliedPrices {
	return &impliedPrices{window: window, denoms: make(map[string]*impliedPrice)}
}

// price returns the USD price of one minimal unit of denom implied by the
// trades in the window before at, or nil if there were none.
func (p *impliedPrices) price(denom string, at time.Time) *osmomath.BigDec {
	entry, ok := p.denoms[denom]
	if !ok {
		return nil
	}

	entry.expire(at.Add(-p.window))
	if len(entry.observations) == 0 {
		delete(p.denoms, denom)
		return nil
	}

	price := entry.usd.Quo(entry.amount)
	return &price
}

// observe records that a trade at at moved amount minimal units of denom for usd.
func (p *impliedPrices) observe(denom string, at time.Time, usd osmomath.BigDec, amount osmomath.Int) {
	if !usd.IsPositive() || !amount.IsPositive() {
		return
	}

	entry, ok := p.denoms[denom]
	if !ok {
		entry = &impliedPrice{usd: osmomath.ZeroBigDec(), amount: osmomath.ZeroBigDec()}
		p.denoms[denom] = entry
	}
	entry.expire(at.Add(-p.window))

	observation := priceObservation{at: at, usd: usd, amount: osmomath.BigDecFromSDKInt(amount)}
	entry.observations = append(entry.observations, observation)
	entry.usd = entry.usd.Add(observation.usd)
	entry.amount = entry.amount.Add(observation.amount)
}

// expire drops the observations before cutoff.
func (e *impliedPrice) expire(cutoff time.Time) {
	expired := 0
	for expired < len(e.observations) && e.observations[expired].at.Before(cutoff) {
		e.usd = e.usd.Sub(e.observations[expired].usd)
		e.amount = e.amount.Sub(e.observations[expired].amount)
		expired++
	}
	if expired == 0 {
		return
	}
	e.observations = e.observations[expired:]
	if len(e.observations) == 0 {
		// Reset exactly rather than keep sums that only cancel out.
		e.observations = nil
		e.usd, e.amount = osmomath.ZeroBigDec(), osmomath.ZeroBigDec()
	}
}
