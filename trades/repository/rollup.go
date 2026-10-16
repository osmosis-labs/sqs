package repository

import (
	"github.com/osmosis-labs/osmosis/osmomath"

	"github.com/osmosis-labs/sqs/domain"
)

// rollup accumulates the aggregates of one scope over one window.
// Every addition is reversed exactly on expiry using the values stored on the
// trade, so the running sums never drift.
type rollup struct {
	buys        int
	sells       int
	pricedBuys  int
	pricedSells int
	buyUsd      osmomath.BigDec
	sellUsd     osmomath.BigDec
	// Wallet -> number of trades in the window, so a wallet stays distinct
	// until its last trade in the window expires.
	buyers  map[string]int
	sellers map[string]int
}

func newRollup() *rollup {
	return &rollup{
		buyUsd:  osmomath.ZeroBigDec(),
		sellUsd: osmomath.ZeroBigDec(),
		buyers:  make(map[string]int),
		sellers: make(map[string]int),
	}
}

// add applies the trade to the rollup when delta is 1 and reverses it when delta is -1.
func (r *rollup) add(trade *domain.Trade, isBuy bool, delta int) {
	count, priced, usd, wallets := &r.sells, &r.pricedSells, &r.sellUsd, r.sellers
	if isBuy {
		count, priced, usd, wallets = &r.buys, &r.pricedBuys, &r.buyUsd, r.buyers
	}

	*count += delta

	if trade.UsdValueAtIngest != nil {
		*priced += delta
		if delta > 0 {
			*usd = usd.Add(*trade.UsdValueAtIngest)
		} else {
			*usd = usd.Sub(*trade.UsdValueAtIngest)
		}
	}

	wallets[trade.Wallet] += delta
	if wallets[trade.Wallet] <= 0 {
		delete(wallets, trade.Wallet)
	}
}

func (r *rollup) isEmpty() bool {
	return r.buys == 0 && r.sells == 0
}

func (r *rollup) toAggregates() domain.TradeWindowAggregates {
	if r == nil {
		return domain.TradeWindowAggregates{}
	}

	result := domain.TradeWindowAggregates{
		Buys:            r.buys,
		Sells:           r.sells,
		DistinctBuyers:  len(r.buyers),
		DistinctSellers: len(r.sellers),
	}

	// A volume is null, not zero, when no trade on that side was priced.
	if r.pricedBuys > 0 {
		v := r.buyUsd.String()
		result.BuyVolumeUsd = &v
	}
	if r.pricedSells > 0 {
		v := r.sellUsd.String()
		result.SellVolumeUsd = &v
	}

	return result
}

// scopeSide reports whether the trade falls in the filter's scope (ignoring the
// wallet) and, if so, whether it counts as a buy.
//
// With a denom, a buy means the trader received that denom and a sell means
// the trader gave it up. Without one, the trade's direction decides, and
// trades with no direction are out of scope.
func scopeSide(trade *domain.Trade, poolID *uint64, denom *string) (isBuy bool, ok bool) {
	if poolID != nil && trade.PoolID != *poolID {
		return false, false
	}

	if denom != nil {
		switch *denom {
		case trade.TokenOut.Denom:
			return true, true
		case trade.TokenIn.Denom:
			return false, true
		default:
			return false, false
		}
	}

	if trade.Direction == nil {
		return false, false
	}

	return *trade.Direction == domain.TradeDirectionBuy, true
}
