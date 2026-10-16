package repository

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/mvc"
	"github.com/osmosis-labs/sqs/trades/telemetry"
)

type poolDenomKey struct {
	poolID uint64
	denom  string
}

// windowState holds the rollups of one window. head is the sequence number of
// the oldest stored trade still inside the window.
type windowState struct {
	window    domain.TradeWindow
	duration  time.Duration
	head      uint64
	pool      map[uint64]*rollup
	poolDenom map[poolDenomKey]*rollup
	denom     map[string]*rollup
}

type tradesRepository struct {
	mu sync.RWMutex

	// trades holds every retained trade ascending by key. trades[i] has
	// sequence number baseSeq+i.
	trades  []*domain.Trade
	baseSeq uint64
	lastKey *domain.TradeKey

	// Secondary indexes, each ascending by key.
	byPool   map[uint64][]*domain.Trade
	byDenom  map[string][]*domain.Trade
	byWallet map[string][]*domain.Trade

	windows   []*windowState
	retention time.Duration

	// denoms interns denom strings so each trade does not hold its own copy.
	denoms map[string]string

	lastIngestedHeight atomic.Int64
}

var _ mvc.TradesRepository = &tradesRepository{}

// New returns an in-memory trades repository that keeps trades for the given
// retention, which must be at least domain.MaxTradeWindowDuration.
func New(retention time.Duration) mvc.TradesRepository {
	if retention < domain.MaxTradeWindowDuration {
		retention = domain.MaxTradeWindowDuration
	}

	windows := make([]*windowState, 0, len(domain.TradeWindows))
	for _, w := range domain.TradeWindows {
		duration, _ := w.Duration()
		windows = append(windows, &windowState{
			window:    w,
			duration:  duration,
			pool:      make(map[uint64]*rollup),
			poolDenom: make(map[poolDenomKey]*rollup),
			denom:     make(map[string]*rollup),
		})
	}

	return &tradesRepository{
		byPool:    make(map[uint64][]*domain.Trade),
		byDenom:   make(map[string][]*domain.Trade),
		byWallet:  make(map[string][]*domain.Trade),
		windows:   windows,
		retention: retention,
		denoms:    make(map[string]string),
	}
}

// StoreTrades implements mvc.TradesRepository.
func (r *tradesRepository) StoreTrades(trades []domain.Trade, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i := range trades {
		if r.lastKey != nil && !r.lastKey.Less(trades[i].Key) {
			// Already stored, or out of order. Keys only move forward.
			continue
		}

		trade := trades[i]
		trade.TokenIn.Denom = r.intern(trade.TokenIn.Denom)
		trade.TokenOut.Denom = r.intern(trade.TokenOut.Denom)

		r.trades = append(r.trades, &trade)
		r.byPool[trade.PoolID] = append(r.byPool[trade.PoolID], &trade)
		r.byDenom[trade.TokenIn.Denom] = append(r.byDenom[trade.TokenIn.Denom], &trade)
		r.byDenom[trade.TokenOut.Denom] = append(r.byDenom[trade.TokenOut.Denom], &trade)
		r.byWallet[trade.Wallet] = append(r.byWallet[trade.Wallet], &trade)

		for _, w := range r.windows {
			w.apply(&trade, 1)
		}

		key := trade.Key
		r.lastKey = &key
	}

	r.expireLocked(now)
}

// Expire implements mvc.TradesRepository.
func (r *tradesRepository) Expire(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireLocked(now)
}

func (r *tradesRepository) expireLocked(now time.Time) {
	endSeq := r.baseSeq + uint64(len(r.trades))

	// Trades are ascending by key and so by block time: expiry walks forward
	// from each window's head and stops at the first trade still inside.
	for _, w := range r.windows {
		cutoff := now.Add(-w.duration)
		for w.head < endSeq {
			trade := r.trades[w.head-r.baseSeq]
			if trade.Timestamp.After(cutoff) {
				break
			}
			w.apply(trade, -1)
			w.head++
		}
	}

	retentionCutoff := now.Add(-r.retention)
	evicted := 0
	for len(r.trades) > 0 {
		trade := r.trades[0]
		if trade.Timestamp.After(retentionCutoff) {
			break
		}

		// Never evict a trade a window still counts. Retention covers the
		// longest window, so this only guards against misconfiguration.
		for _, w := range r.windows {
			if w.head <= r.baseSeq {
				return
			}
		}

		popFront(r.byPool, trade.PoolID, trade)
		popFront(r.byDenom, trade.TokenIn.Denom, trade)
		popFront(r.byDenom, trade.TokenOut.Denom, trade)
		popFront(r.byWallet, trade.Wallet, trade)

		r.trades[0] = nil
		r.trades = r.trades[1:]
		r.baseSeq++
		evicted++
	}

	if evicted > 0 {
		telemetry.RepoEvictedCounter.Add(float64(evicted))
	}
}

// popFront removes trade from the front of index[key]. Eviction is oldest
// first, so the trade is always at the front of each of its index slices.
func popFront[K comparable](index map[K][]*domain.Trade, key K, trade *domain.Trade) {
	s := index[key]
	if len(s) == 0 || s[0] != trade {
		return
	}
	s[0] = nil
	s = s[1:]
	if len(s) == 0 {
		delete(index, key)
		return
	}
	index[key] = s
}

func (r *tradesRepository) intern(denom string) string {
	if interned, ok := r.denoms[denom]; ok {
		return interned
	}
	r.denoms[denom] = denom
	return denom
}

// apply adds (delta 1) or removes (delta -1) the trade's contributions to every
// rollup it participates in.
func (w *windowState) apply(trade *domain.Trade, delta int) {
	if trade.Direction != nil {
		applyTo(w.pool, trade.PoolID, trade, *trade.Direction == domain.TradeDirectionBuy, delta)
	}

	// The trader received TokenOut (a buy of it) and gave up TokenIn (a sell of it).
	applyTo(w.poolDenom, poolDenomKey{poolID: trade.PoolID, denom: trade.TokenOut.Denom}, trade, true, delta)
	applyTo(w.poolDenom, poolDenomKey{poolID: trade.PoolID, denom: trade.TokenIn.Denom}, trade, false, delta)
	applyTo(w.denom, trade.TokenOut.Denom, trade, true, delta)
	applyTo(w.denom, trade.TokenIn.Denom, trade, false, delta)
}

func applyTo[K comparable](rollups map[K]*rollup, key K, trade *domain.Trade, isBuy bool, delta int) {
	r, ok := rollups[key]
	if !ok {
		if delta < 0 {
			return
		}
		r = newRollup()
		rollups[key] = r
	}

	r.add(trade, isBuy, delta)

	if r.isEmpty() {
		delete(rollups, key)
	}
}

// ListTrades implements mvc.TradesRepository.
func (r *tradesRepository) ListTrades(filter domain.TradesFilter, cursor *domain.TradeKey, limit int) ([]domain.Trade, *domain.TradeKey) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	candidates, ok := r.smallestIndex(filter)
	if !ok || limit <= 0 {
		return []domain.Trade{}, nil
	}

	// Start just below the cursor, or at the newest trade.
	start := len(candidates)
	if cursor != nil {
		start = sort.Search(len(candidates), func(i int) bool {
			return !candidates[i].Key.Less(*cursor)
		})
	}

	result := make([]domain.Trade, 0, limit)
	for i := start - 1; i >= 0; i-- {
		trade := candidates[i]
		if !matches(trade, filter) {
			continue
		}

		if len(result) == limit {
			// There is at least one more match, so hand out a cursor.
			next := result[len(result)-1].Key
			return result, &next
		}

		result = append(result, *trade)
	}

	return result, nil
}

// smallestIndex returns the shortest index slice among the filter's fields.
// Returns false if the filter has no indexed field.
func (r *tradesRepository) smallestIndex(filter domain.TradesFilter) ([]*domain.Trade, bool) {
	var (
		best  []*domain.Trade
		found bool
	)

	consider := func(s []*domain.Trade) {
		if !found || len(s) < len(best) {
			best, found = s, true
		}
	}

	if filter.PoolID != nil {
		consider(r.byPool[*filter.PoolID])
	}
	if filter.Denom != nil {
		consider(r.byDenom[*filter.Denom])
	}
	if filter.Wallet != nil {
		consider(r.byWallet[*filter.Wallet])
	}

	return best, found
}

func matches(trade *domain.Trade, filter domain.TradesFilter) bool {
	if filter.PoolID != nil && trade.PoolID != *filter.PoolID {
		return false
	}
	if filter.Denom != nil && trade.TokenIn.Denom != *filter.Denom && trade.TokenOut.Denom != *filter.Denom {
		return false
	}
	if filter.Wallet != nil && trade.Wallet != *filter.Wallet {
		return false
	}
	return true
}

// GetAggregates implements mvc.TradesRepository.
func (r *tradesRepository) GetAggregates(filter domain.TradesFilter, windows []domain.TradeWindow, now time.Time) map[domain.TradeWindow]domain.TradeWindowAggregates {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Expire first so the windows end exactly at now rather than at the last block.
	r.expireLocked(now)

	result := make(map[domain.TradeWindow]domain.TradeWindowAggregates, len(windows))

	if filter.Wallet != nil {
		for _, w := range windows {
			result[w] = r.walletAggregates(filter, w, now)
		}
		return result
	}

	for _, w := range windows {
		state := r.windowState(w)
		if state == nil {
			continue
		}

		var rollup *rollup
		switch {
		case filter.PoolID != nil && filter.Denom != nil:
			rollup = state.poolDenom[poolDenomKey{poolID: *filter.PoolID, denom: *filter.Denom}]
		case filter.PoolID != nil:
			rollup = state.pool[*filter.PoolID]
		case filter.Denom != nil:
			rollup = state.denom[*filter.Denom]
		}

		result[w] = rollup.toAggregates()
	}

	return result
}

// walletAggregates computes a wallet's aggregates from its own trades. The
// per-wallet index keeps this proportional to that wallet's activity, so
// there is no need for precomputed per-wallet rollups.
func (r *tradesRepository) walletAggregates(filter domain.TradesFilter, window domain.TradeWindow, now time.Time) domain.TradeWindowAggregates {
	duration, ok := window.Duration()
	if !ok {
		return domain.TradeWindowAggregates{}
	}
	cutoff := now.Add(-duration)

	acc := newRollup()
	trades := r.byWallet[*filter.Wallet]
	for i := len(trades) - 1; i >= 0; i-- {
		trade := trades[i]
		if !trade.Timestamp.After(cutoff) {
			break
		}
		isBuy, ok := scopeSide(trade, filter.PoolID, filter.Denom)
		if !ok {
			continue
		}
		acc.add(trade, isBuy, 1)
	}

	return acc.toAggregates()
}

func (r *tradesRepository) windowState(w domain.TradeWindow) *windowState {
	for _, state := range r.windows {
		if state.window == w {
			return state
		}
	}
	return nil
}

// LastIngestedHeight implements mvc.TradesRepository.
func (r *tradesRepository) LastIngestedHeight() int64 {
	return r.lastIngestedHeight.Load()
}

// SetLastIngestedHeight implements mvc.TradesRepository.
func (r *tradesRepository) SetLastIngestedHeight(height int64) {
	r.lastIngestedHeight.Store(height)
}
