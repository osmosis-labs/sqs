package repository

import (
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
)

const (
	uosmo  = "uosmo"
	usdc   = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
	atom   = "ibc/27394FB092D2ECCD56123C74F36E4C1F926001CEADA9CA97EA622B25F41E5EB2"
	allBTC = "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"

	walletA = "osmo1l3et2hda5px5r60h8xe5cs49ettdvm9sdn2m6d"
	walletB = "osmo17qdmjdumw4xawam4g46gtwzle5rd4zwyfqvvza"
)

var now = time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)

func usd(v string) *osmomath.BigDec {
	d := osmomath.MustNewBigDecFromStr(v)
	return &d
}

func dir(d domain.TradeDirection) *domain.TradeDirection {
	return &d
}

func strPtr(s string) *string { return &s }

func u64(v uint64) *uint64 { return &v }

func newTrade(height int64, txIndex, eventIndex uint32, age time.Duration, poolID uint64, in, out string, wallet string, value *osmomath.BigDec, direction *domain.TradeDirection) domain.Trade {
	return domain.Trade{
		Key:              domain.TradeKey{BlockHeight: height, TxIndex: txIndex, EventIndex: eventIndex},
		Timestamp:        now.Add(-age),
		TxHash:           "TX",
		Wallet:           wallet,
		PoolID:           poolID,
		TokenIn:          sdk.NewInt64Coin(in, 1_000_000),
		TokenOut:         sdk.NewInt64Coin(out, 2_000_000),
		UsdValueAtIngest: value,
		Direction:        direction,
		Source:           domain.TradeSourceSwap,
	}
}

// Trades leave each window exactly when they age past it, decremented by the
// value stored at ingest, and leave memory after the retention period.
func TestWindowExpiry(t *testing.T) {
	repo := New(8 * 24 * time.Hour)

	repo.StoreTrades([]domain.Trade{
		newTrade(1, 0, 0, 24*time.Hour+time.Minute, 1, usdc, uosmo, walletA, usd("10"), dir(domain.TradeDirectionBuy)),
		newTrade(2, 0, 0, 24*time.Hour-time.Minute, 1, usdc, uosmo, walletA, usd("20"), dir(domain.TradeDirectionBuy)),
		newTrade(3, 0, 0, time.Hour, 1, uosmo, usdc, walletB, usd("5"), dir(domain.TradeDirectionSell)),
		newTrade(4, 0, 0, time.Hour, 1, uosmo, usdc, walletB, nil, dir(domain.TradeDirectionSell)),
	}, now)

	poolScope := domain.TradesFilter{PoolID: u64(1)}

	got := repo.GetAggregates(poolScope, domain.TradeWindows, now)
	require.Equal(t, domain.TradeWindowAggregates{
		Buys: 1, Sells: 2, BuyVolumeUsd: strPtr(osmomath.MustNewBigDecFromStr("20").String()), SellVolumeUsd: strPtr(osmomath.MustNewBigDecFromStr("5").String()),
		DistinctBuyers: 1, DistinctSellers: 1,
	}, got[domain.TradeWindow24h])
	require.Equal(t, 2, got[domain.TradeWindow7d].Buys)
	require.Equal(t, osmomath.MustNewBigDecFromStr("30").String(), *got[domain.TradeWindow7d].BuyVolumeUsd)

	// Two minutes later the 23h59m trade has left the 24h window but not the 7d one.
	later := now.Add(2 * time.Minute)
	got = repo.GetAggregates(poolScope, domain.TradeWindows, later)
	require.Equal(t, 0, got[domain.TradeWindow24h].Buys)
	require.Nil(t, got[domain.TradeWindow24h].BuyVolumeUsd, "no priced buys left: null, not zero")
	require.Equal(t, 0, got[domain.TradeWindow24h].DistinctBuyers)
	require.Equal(t, 2, got[domain.TradeWindow7d].Buys)

	// Past retention everything is gone, from the windows and from the list.
	muchLater := now.Add(9 * 24 * time.Hour)
	got = repo.GetAggregates(poolScope, domain.TradeWindows, muchLater)
	require.Equal(t, domain.TradeWindowAggregates{}, got[domain.TradeWindow7d])
	trades, next := repo.ListTrades(poolScope, nil, 10)
	require.Empty(t, trades)
	require.Nil(t, next)
}

// The incremental rollups equal a from-scratch computation over the stored
// trades for every scope, before and after expiry, and the per-pool denom
// rollups sum to the asset-wide rollup.
func TestRollupsMatchRecompute(t *testing.T) {
	repo := New(8 * 24 * time.Hour).(*tradesRepository)

	pools := []uint64{1, 2, 3, 4, 5}
	others := []string{usdc, atom, allBTC}
	wallets := []string{walletA, walletB}

	var trades []domain.Trade
	height := int64(1)
	for i := 0; i < 200; i++ {
		other := others[i%len(others)]
		in, out := uosmo, other
		direction := dir(domain.TradeDirectionSell)
		if i%2 == 0 {
			in, out = other, uosmo
			direction = dir(domain.TradeDirectionBuy)
		}
		if other == allBTC {
			direction = nil
		}
		var value *osmomath.BigDec
		if i%5 != 0 {
			value = usd("1.000000000000000001")
		}
		// Spread over 8 days, oldest first, two events per tx as in a multi-hop swap.
		age := time.Duration(200-i) * 50 * time.Minute
		trades = append(trades, newTrade(height, 0, uint32(i%2), age, pools[i%len(pools)], in, out, wallets[i%len(wallets)], value, direction))
		if i%2 == 1 {
			height++
		}
	}
	repo.StoreTrades(trades, now)

	check := func(at time.Time) {
		scopes := []domain.TradesFilter{
			{Denom: strPtr(uosmo)},
			{Denom: strPtr(allBTC)},
			{Denom: strPtr(uosmo), Wallet: strPtr(walletA)},
			{PoolID: u64(2), Wallet: strPtr(walletB)},
		}
		for _, p := range pools {
			scopes = append(scopes, domain.TradesFilter{PoolID: u64(p)}, domain.TradesFilter{PoolID: u64(p), Denom: strPtr(uosmo)})
		}

		for _, scope := range scopes {
			got := repo.GetAggregates(scope, domain.TradeWindows, at)
			for _, w := range domain.TradeWindows {
				duration, _ := w.Duration()
				want := newRollup()
				for _, trade := range repo.trades {
					if !trade.Timestamp.After(at.Add(-duration)) || (scope.Wallet != nil && trade.Wallet != *scope.Wallet) {
						continue
					}
					if isBuy, ok := scopeSide(trade, scope.PoolID, scope.Denom); ok {
						want.add(trade, isBuy, 1)
					}
				}
				require.Equal(t, want.toAggregates(), got[w], "window %s scope %+v", w, scope)
			}
		}

		// Per-pool OSMO rollups sum to the asset-wide OSMO rollup.
		for _, w := range domain.TradeWindows {
			asset := repo.GetAggregates(domain.TradesFilter{Denom: strPtr(uosmo)}, []domain.TradeWindow{w}, at)[w]
			buys, sells := 0, 0
			for _, p := range pools {
				perPool := repo.GetAggregates(domain.TradesFilter{PoolID: u64(p), Denom: strPtr(uosmo)}, []domain.TradeWindow{w}, at)[w]
				buys += perPool.Buys
				sells += perPool.Sells
			}
			require.Equal(t, asset.Buys, buys)
			require.Equal(t, asset.Sells, sells)
		}
	}

	check(now)
	check(now.Add(3 * 24 * time.Hour))
}

// Pages are newest first with no duplicates or gaps across the events of one
// tx, and re-storing an already stored block is a no-op.
func TestListTradesPagination(t *testing.T) {
	repo := New(8 * 24 * time.Hour)

	block := []domain.Trade{
		newTrade(10, 0, 3, time.Minute, 1, usdc, uosmo, walletA, nil, nil),
		newTrade(10, 0, 9, time.Minute, 2, uosmo, atom, walletA, nil, nil),
		newTrade(10, 1, 2, time.Minute, 1, uosmo, usdc, walletB, nil, nil),
		newTrade(11, 0, 1, 0, 2, atom, uosmo, walletB, nil, nil),
		newTrade(11, 0, 4, 0, 1, usdc, uosmo, walletA, nil, nil),
	}
	repo.StoreTrades(block, now)
	repo.StoreTrades(block, now)

	osmoScope := domain.TradesFilter{Denom: strPtr(uosmo)}

	var (
		keys   []domain.TradeKey
		cursor *domain.TradeKey
	)
	for page := 0; page < 10; page++ {
		trades, next := repo.ListTrades(osmoScope, cursor, 2)
		for _, trade := range trades {
			keys = append(keys, trade.Key)
		}
		if next == nil {
			break
		}
		cursor = next
	}

	require.Equal(t, []domain.TradeKey{
		{BlockHeight: 11, TxIndex: 0, EventIndex: 4},
		{BlockHeight: 11, TxIndex: 0, EventIndex: 1},
		{BlockHeight: 10, TxIndex: 1, EventIndex: 2},
		{BlockHeight: 10, TxIndex: 0, EventIndex: 9},
		{BlockHeight: 10, TxIndex: 0, EventIndex: 3},
	}, keys)

	// Filters combine: wallet A in pool 1.
	trades, next := repo.ListTrades(domain.TradesFilter{PoolID: u64(1), Wallet: strPtr(walletA)}, nil, 10)
	require.Nil(t, next)
	require.Len(t, trades, 2)
	require.Equal(t, domain.TradeKey{BlockHeight: 11, TxIndex: 0, EventIndex: 4}, trades[0].Key)
	require.Equal(t, domain.TradeKey{BlockHeight: 10, TxIndex: 0, EventIndex: 3}, trades[1].Key)
}
