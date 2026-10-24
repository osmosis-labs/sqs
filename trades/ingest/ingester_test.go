package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/osmosis-labs/osmosis/osmomath"
	"github.com/osmosis-labs/osmosis/v31/ingest/types/cosmwasmpool"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
	"github.com/osmosis-labs/sqs/domain/mvc"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/log"
	"github.com/osmosis-labs/sqs/trades/repository"
	"github.com/osmosis-labs/sqs/trades/telemetry"
)

const (
	uosmo   = "uosmo"
	usdc    = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4" // USDC.noble
	usdcAxl = "ibc/D189335C6E4A68B513C10AB227BF1C1D38C746766278BA3EEB4FB14124F1D858"
	allBTC  = "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"
	other   = "ibc/164807F6226F91990F358C6467EEE8B162E437BDCD3DADEC3F0CE20693720795"

	fixtureHeight = int64(72275545)
)

type fakePools map[uint64]ingesttypes.PoolI

func (f fakePools) GetPool(poolID uint64) (ingesttypes.PoolI, error) {
	pool, ok := f[poolID]
	if !ok {
		return nil, domain.PoolNotFoundError{PoolID: poolID}
	}
	return pool, nil
}

type fakePricing map[string]osmomath.BigDec

func (f fakePricing) GetPrice(_ context.Context, baseDenom, _ string, _ ...domain.PricingOption) (osmomath.BigDec, []domain.SplitRoute, error) {
	price, ok := f[baseDenom]
	if !ok {
		return osmomath.BigDec{}, nil, errors.New("no route")
	}
	return price, nil, nil
}

func (fakePricing) InitializeCache(*cache.Cache) {}

func (fakePricing) GetFallbackStrategy(string) domain.PricingSourceType {
	return domain.NoneSourceType
}

func pool(id uint64, poolType poolmanagertypes.PoolType, model *cosmwasmpool.CosmWasmPoolModel) ingesttypes.PoolI {
	return &mocks.MockRoutablePool{ID: id, PoolType: poolType, CosmWasmPoolModel: model}
}

func orderbookModel(base, quote string) *cosmwasmpool.CosmWasmPoolModel {
	return &cosmwasmpool.CosmWasmPoolModel{
		ContractInfo: cosmwasmpool.ContractInfo{Contract: cosmwasmpool.ORDERBOOK_CONTRACT_NAME, Version: "2.0.0"},
		Data:         cosmwasmpool.CosmWasmPoolData{Orderbook: &cosmwasmpool.OrderbookData{BaseDenom: base, QuoteDenom: quote}},
	}
}

func alloyModel() *cosmwasmpool.CosmWasmPoolModel {
	return &cosmwasmpool.CosmWasmPoolModel{
		ContractInfo: cosmwasmpool.ContractInfo{Contract: cosmwasmpool.ALLOY_TRANSMUTER_CONTRACT_NAME, Version: cosmwasmpool.ALLOY_TRANSMUTER_MIN_CONTRACT_VERSION},
		Data:         cosmwasmpool.CosmWasmPoolData{AlloyTransmuter: &cosmwasmpool.AlloyTransmuterData{AlloyedDenom: allBTC}},
	}
}

func sixDecimals(string) (osmomath.Dec, error) {
	return osmomath.NewDec(1_000_000), nil
}

func newTestNormaliser(pools fakePools) *normaliser {
	pricing := fakePricing{uosmo: osmomath.MustNewBigDecFromStr("0.0251")}
	return newNormaliser(pools, pricing, usdc, sixDecimals, domain.DefaultConfig.Trades.QuoteDenoms)
}

func bigDec(s string) *osmomath.BigDec {
	d := osmomath.MustNewBigDecFromStr(s)
	return &d
}

func direction(d domain.TradeDirection) *domain.TradeDirection {
	return &d
}

// loadFixture loads txs 0-4 of mainnet block 72275545 (block_results trimmed
// to code and events). Tx 2 is a split-route swap through orderbook pool 1933
// followed by protorev back-runs; tx 4 is an authz-executed contract swap.
func loadFixture(t *testing.T) BlockData {
	raw, err := os.ReadFile("testdata/block_results_72275545.json")
	require.NoError(t, err)

	var results struct {
		TxsResults []*abcitypes.ExecTxResult `json:"txs_results"`
	}
	require.NoError(t, json.Unmarshal(raw, &results))

	return BlockData{
		Height: fixtureHeight,
		Time:   time.Date(2026, 10, 10, 15, 59, 47, 378587916, time.UTC),
		TxHashes: []string{
			"7306A403B0B1AAB73AA4A6FAFA26366DC56FF957E47B059A85685C4997414142",
			"1CFA5CF9DD50C861C3E7CB43E3E571E042BE3BAD4863F79FCFED9FEC6392D1CF",
			"524CABC86F1A20EB86C1056F306435186C07FEE51D7F5ED1AE26B68978773DF1",
			"1ACED7C1301F0631672CB440E0647EA910E9DF676FC17BB35A2D7B0E50D613B4",
			"A4DFAE06E439B1A87D098A6C326C2137D5E8DFED4C4A89C50022705E1A905F2E",
		},
		TxResults: results.TxsResults,
	}
}

func TestExtractTradesFromMainnetBlock(t *testing.T) {
	block := loadFixture(t)

	pools := fakePools{
		1933: pool(1933, poolmanagertypes.CosmWasm, orderbookModel(uosmo, usdc)),
		1464: pool(1464, poolmanagertypes.Concentrated, nil),
		1943: pool(1943, poolmanagertypes.Concentrated, nil),
		2262: pool(2262, poolmanagertypes.Concentrated, nil),
		2263: pool(2263, poolmanagertypes.Concentrated, nil),
		2265: pool(2265, poolmanagertypes.Balancer, nil),
		1223: pool(1223, poolmanagertypes.Stableswap, nil),
		// Pools 3, 678, 2286, 2393 and 2401 are unknown to SQS and skipped.
	}

	ingester := &Ingester{normaliser: newTestNormaliser(pools), logger: &log.NoOpLogger{}}
	trades, err := ingester.extractTrades(context.Background(), block, true)
	require.NoError(t, err)

	type summary struct {
		key       domain.TradeKey
		poolID    uint64
		wallet    string
		in, out   string
		usd       string // empty when unpriced
		direction string // empty when null
		source    domain.TradeSource
	}
	key := func(tx, event uint32) domain.TradeKey {
		return domain.TradeKey{BlockHeight: fixtureHeight, TxIndex: tx, EventIndex: event}
	}
	coin := func(amount int64, denom string) string { return sdk.NewInt64Coin(denom, amount).String() }
	trader := "osmo1l3et2hda5px5r60h8xe5cs49ettdvm9sdn2m6d"
	protorev := "osmo17qdmjdumw4xawam4g46gtwzle5rd4zwyfqvvza"
	contract := "osmo13p627xdgllr38wpw8r4swsn0prn4kh3nyhajakxuxtejdsdhky4qx38fj6"
	buy, sell := string(domain.TradeDirectionBuy), string(domain.TradeDirectionSell)
	usd := func(v string) string { return osmomath.MustNewBigDecFromStr(v).String() }
	swap, fill := domain.TradeSourceSwap, domain.TradeSourceOrderbookFill

	want := []summary{
		// Tx 2: split route; the second hop fills against the OSMO/USDC orderbook.
		{key(2, 25), 2262, trader, coin(697203, usdc), coin(22346003, other), usd("0.697203"), buy, swap},
		{key(2, 39), 1933, trader, coin(99900, usdc), coin(2516372, uosmo), usd("0.0999"), buy, fill},
		{key(2, 52), 2265, trader, coin(2513855, uosmo), coin(3121287, other), usd("0.0630977605"), buy, swap},
		{key(2, 69), 1943, trader, coin(199781, usdc), coin(269, allBTC), usd("0.199781"), buy, swap},
		// Neither denom is a quote: no direction, and allBTC (lexicographically first) has no price.
		{key(2, 82), 2263, trader, coin(268, allBTC), coin(6537774, other), "", "", swap},
		// Protorev back-run in the same tx, attributed to the protorev module.
		{key(2, 94), 2265, protorev, coin(14000000, uosmo), coin(16598198, other), usd("0.3514"), buy, swap},
		{key(2, 107), 2262, protorev, coin(16565002, other), coin(522089, usdc), usd("0.522089"), sell, swap},
		{key(2, 120), 1464, protorev, coin(522037, usdc), coin(15000339, uosmo), usd("0.522037"), buy, swap},
		// Tx 4: swaps routed by a contract carry the contract as sender.
		{key(4, 70), 2262, contract, coin(5411245, other), coin(164049, usdc), usd("0.164049"), sell, swap},
		{key(4, 83), 1223, contract, coin(164033, usdc), coin(164016, usdcAxl), usd("0.164033"), "", swap},
		{key(4, 116), 1464, protorev, coin(999900, uosmo), coin(34800, usdc), usd("0.0348"), sell, swap},
		{key(4, 129), 2262, protorev, coin(34731, usdc), coin(1154138, other), usd("0.034731"), buy, swap},
		{key(4, 138), 2265, protorev, coin(1154138, other), coin(1005456, uosmo), usd("0.0252369456"), sell, swap},
	}

	got := make([]summary, 0, len(trades))
	for _, trade := range trades {
		require.Equal(t, block.Time, trade.Timestamp)
		require.Equal(t, block.TxHashes[trade.Key.TxIndex], trade.TxHash)
		row := summary{key: trade.Key, poolID: trade.PoolID, wallet: trade.Wallet, in: trade.TokenIn.String(), out: trade.TokenOut.String(), source: trade.Source}
		if trade.UsdValueAtIngest != nil {
			row.usd = trade.UsdValueAtIngest.String()
		}
		if trade.Direction != nil {
			row.direction = string(*trade.Direction)
		}
		got = append(got, row)
	}
	require.Equal(t, want, got)
}

func swapEventWith(attributes ...string) abcitypes.Event {
	event := abcitypes.Event{Type: eventTypeTokenSwapped}
	for i := 0; i+1 < len(attributes); i += 2 {
		event.Attributes = append(event.Attributes, abcitypes.EventAttribute{Key: attributes[i], Value: attributes[i+1]})
	}
	return event
}

func TestNormalise(t *testing.T) {
	pools := fakePools{
		1933: pool(1933, poolmanagertypes.CosmWasm, orderbookModel(uosmo, usdc)),
		1868: pool(1868, poolmanagertypes.CosmWasm, alloyModel()),
		999:  pool(999, poolmanagertypes.CosmWasm, nil),
		1:    pool(1, poolmanagertypes.Balancer, nil),
	}
	n := newTestNormaliser(pools)

	tests := []struct {
		name          string
		event         abcitypes.Event
		wantSkip      string
		wantDirection *domain.TradeDirection
		wantSource    domain.TradeSource
		wantUsd       *osmomath.BigDec
	}{
		{
			name:          "orderbook ask filled: taker sells the base",
			event:         swapEventWith("sender", "osmo1a", "pool_id", "1933", "tokens_in", "1000000uosmo", "tokens_out", "25000"+usdc),
			wantDirection: direction(domain.TradeDirectionSell),
			wantSource:    domain.TradeSourceOrderbookFill,
			wantUsd:       bigDec("0.025"),
		},
		{
			name:       "alloyed transmuter has no direction",
			event:      swapEventWith("sender", "osmo1a", "pool_id", "1868", "tokens_in", "100"+allBTC, "tokens_out", "100ibc/D1542AA8762DB13087D8364F3EA6509FD6F009A34F00426AF9E4F9FA85CBBF1F"),
			wantSource: domain.TradeSourceSwap,
		},
		{
			name:     "unsupported cosmwasm pool",
			event:    swapEventWith("sender", "osmo1a", "pool_id", "999", "tokens_in", "1uosmo", "tokens_out", "1"+usdc),
			wantSkip: telemetry.SkipReasonPoolType,
		},
		{
			name:     "unknown pool",
			event:    swapEventWith("sender", "osmo1a", "pool_id", "5", "tokens_in", "1uosmo", "tokens_out", "1"+usdc),
			wantSkip: telemetry.SkipReasonPoolNotFound,
		},
		{
			name:     "missing tokens_in",
			event:    swapEventWith("sender", "osmo1a", "pool_id", "1", "tokens_out", "1"+usdc),
			wantSkip: telemetry.SkipReasonMalformed,
		},
		{
			name:     "multi-coin tokens_in",
			event:    swapEventWith("sender", "osmo1a", "pool_id", "1", "tokens_in", "1uatom,1uosmo", "tokens_out", "1"+usdc),
			wantSkip: telemetry.SkipReasonMalformed,
		},
		{
			name:     "duplicate pool_id",
			event:    swapEventWith("sender", "osmo1a", "pool_id", "1", "pool_id", "2", "tokens_in", "1uosmo", "tokens_out", "1"+usdc),
			wantSkip: telemetry.SkipReasonMalformed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trade, skip, err := n.normalise(context.Background(), tc.event, domain.TradeKey{BlockHeight: 1}, time.Unix(0, 0), "TX", make(priceCache), true, nil)
			require.Equal(t, tc.wantSkip, skip)
			require.Equal(t, tc.wantSkip == telemetry.SkipReasonMalformed, err != nil)
			if tc.wantSkip != "" {
				return
			}
			require.Equal(t, tc.wantDirection, trade.Direction)
			require.Equal(t, tc.wantSource, trade.Source)
			if tc.wantUsd == nil {
				require.Nil(t, trade.UsdValueAtIngest)
				return
			}
			require.NotNil(t, trade.UsdValueAtIngest)
			require.Equal(t, tc.wantUsd.String(), trade.UsdValueAtIngest.String())
		})
	}
}

// A backfilled trade is valued at its time: exactly from a leg in the USD quote
// denom, otherwise from the price its legs' recent trades imply, chaining from
// the USD quote denom through OSMO, at most two trades deep. The spot price is
// never used.
func TestTradeTimeValue(t *testing.T) {
	pools := fakePools{1: pool(1, poolmanagertypes.Balancer, nil)}
	n := newTestNormaliser(pools) // spot OSMO is 0.0251
	history := newTradeTimePrices(impliedPriceWindow)
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

	value := func(at time.Duration, tokensIn, tokensOut string) *osmomath.BigDec {
		event := swapEventWith("sender", "osmo1a", "pool_id", "1", "tokens_in", tokensIn, "tokens_out", tokensOut)
		trade, skip, err := n.normalise(context.Background(), event, domain.TradeKey{BlockHeight: 1}, start.Add(at), "TX", make(priceCache), false, history)
		require.NoError(t, err)
		require.Empty(t, skip)
		return trade.UsdValueAtIngest
	}
	requireUsd := func(want string, got *osmomath.BigDec) {
		t.Helper()
		require.NotNil(t, got)
		require.Equal(t, osmomath.MustNewBigDecFromStr(want).String(), got.String())
	}

	// No leg in the USD quote denom and no history yet.
	require.Nil(t, value(0, "2000000"+other, "1000000uosmo"))

	// The USD quote leg values the trade exactly, at 0.03 per OSMO rather than
	// the spot 0.0251, and two trades imply a volume-weighted OSMO price.
	requireUsd("30", value(time.Minute, "1000000000uosmo", "30000000"+usdc))
	requireUsd("10", value(2*time.Minute, "10000000"+usdc, "250000000uosmo"))
	require.Equal(t, osmomath.MustNewBigDecFromStr("0.000000032").String(), history.direct.price(uosmo, start.Add(2*time.Minute)).String())

	// Priced through OSMO: 500 OSMO at 0.032, giving other a derived price.
	requireUsd("16", value(3*time.Minute, "200000000"+other, "500000000uosmo"))

	// Neither leg is a quote denom: allBTC, the first leg tried, has no price,
	// so other values it at the 0.08 its OSMO trade implied. A trade valued at
	// a derived price gives no price itself.
	requireUsd("4", value(4*time.Minute, "50000000"+other, "1000"+allBTC))
	require.Nil(t, history.derived.price(allBTC, start.Add(4*time.Minute)))
	require.Nil(t, history.direct.price(allBTC, start.Add(4*time.Minute)))

	// Once those trades are older than the window, nothing prices other.
	require.Nil(t, value(4*time.Minute+impliedPriceWindow+time.Second, "1000000"+other, "1000000uosmo"))
}

// A round trip through another asset cannot re-anchor a price on itself once
// its direct price has expired, which would turn fees into a price rise.
func TestTradeTimeValueNoFeedback(t *testing.T) {
	n := newTestNormaliser(fakePools{1: pool(1, poolmanagertypes.Balancer, nil)})
	history := newTradeTimePrices(impliedPriceWindow)
	start := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	value := func(at time.Duration, tokensIn, tokensOut string) *osmomath.BigDec {
		event := swapEventWith("sender", "osmo1a", "pool_id", "1", "tokens_in", tokensIn, "tokens_out", tokensOut)
		trade, _, err := n.normalise(context.Background(), event, domain.TradeKey{BlockHeight: 1}, start.Add(at), "TX", make(priceCache), false, history)
		require.NoError(t, err)
		return trade.UsdValueAtIngest
	}

	// OSMO at $1, then 100 OSMO swapped for 80 other: other is derived at $1.25.
	require.NotNil(t, value(0, "1000000uosmo", "1000000"+usdc))
	require.NotNil(t, value(14*time.Minute, "100000000uosmo", "80000000"+other))

	// After OSMO's direct price expires, 80 other back to 64 OSMO is valued at
	// $100 from other's derived price, but does not price OSMO at $100/64.
	got := value(16*time.Minute, "80000000"+other, "64000000uosmo")
	require.NotNil(t, got)
	require.Equal(t, osmomath.MustNewBigDecFromStr("100").String(), got.String())
	require.Nil(t, history.direct.price(uosmo, start.Add(16*time.Minute)))
	require.Nil(t, history.derived.price(uosmo, start.Add(16*time.Minute)))
}

func TestImpliedPricesWindow(t *testing.T) {
	history := newImpliedPrices(time.Minute)
	start := time.Unix(1_000_000, 0)

	history.observe(uosmo, start, osmomath.NewBigDec(2), osmomath.NewInt(100))
	history.observe(uosmo, start.Add(30*time.Second), osmomath.NewBigDec(6), osmomath.NewInt(100))
	require.Equal(t, osmomath.MustNewBigDecFromStr("0.04").String(), history.price(uosmo, start.Add(time.Minute)).String())

	// The first observation has left the window.
	require.Equal(t, osmomath.MustNewBigDecFromStr("0.06").String(), history.price(uosmo, start.Add(61*time.Second)).String())
	require.Nil(t, history.price(uosmo, start.Add(91*time.Second)))
	require.Nil(t, history.price(usdc, start))

	// Zero values are not observed.
	history.observe(uosmo, start.Add(2*time.Minute), osmomath.ZeroBigDec(), osmomath.NewInt(100))
	require.Nil(t, history.price(uosmo, start.Add(2*time.Minute)))
}

// An 18-decimal amount is valued exactly in BigDec, with no int64 or Dec truncation.
func TestUsdValueEighteenDecimals(t *testing.T) {
	const weth = "ibc/EA1D43981D5C9A1C4AAEA9C23BB1D4FA126BA9BC7020A25E0AE4AA841EA25DC5"
	n := newNormaliser(fakePools{}, fakePricing{weth: osmomath.MustNewBigDecFromStr("2500.5")}, usdc,
		func(string) (osmomath.Dec, error) {
			return osmomath.NewDecWithPrec(1, 0).MulInt64(1_000_000_000_000_000_000), nil
		}, nil)

	amount, ok := osmomath.NewIntFromString("1234567890123456789012345")
	require.True(t, ok)

	got := n.usdValue(context.Background(), sdk.NewCoin(weth, amount), make(priceCache))
	require.NotNil(t, got)
	require.Equal(t, osmomath.MustNewBigDecFromStr("3087037009.2537037009253686725").String(), got.String())

	// Unpriced and zero-priced denoms are null, never zero.
	n.pricing = fakePricing{weth: osmomath.ZeroBigDec()}
	require.Nil(t, n.usdValue(context.Background(), sdk.NewCoin(weth, amount), make(priceCache)))
}

// fakeFetcher serves empty blocks, or with swaps one orderbook fill on pool
// 1933 per block, and reports earliest as the node's oldest kept block.
type fakeFetcher struct {
	failAt      map[int64]bool
	fetched     []int64
	swaps       bool
	earliest    int64
	earliestErr error
	onFetch     func(height int64)
}

func (f *fakeFetcher) FetchBlock(_ context.Context, height int64) (BlockData, error) {
	if f.onFetch != nil {
		f.onFetch(height)
	}
	if f.failAt[height] {
		return BlockData{}, errors.New("rpc unavailable")
	}
	f.fetched = append(f.fetched, height)
	block := BlockData{Height: height, Time: time.Now()}
	if f.swaps {
		event := swapEventWith("sender", "osmo1a", "pool_id", "1933", "tokens_in", "1000000uosmo", "tokens_out", "25000"+usdc)
		block.TxHashes = []string{"TX"}
		block.TxResults = []*abcitypes.ExecTxResult{{Events: []abcitypes.Event{event}}}
	}
	return block, nil
}

func (f *fakeFetcher) EarliestHeight(context.Context) (int64, error) {
	return f.earliest, f.earliestErr
}

// Heights are processed in order, forward-only from the first signal, failed
// heights are retried from where they stopped until they have failed
// maxHeightAttempts times, and a lag beyond the catch-up bound skips the
// oldest blocks.
func TestIngesterCatchUp(t *testing.T) {
	fetcher := &fakeFetcher{failAt: map[int64]bool{}}
	repo := repository.New(8 * 24 * time.Hour)
	ingester := NewIngester(fetcher, fakePools{}, fakePricing{}, usdc, sixDecimals, nil, repo, 3, 0, &log.NoOpLogger{})
	ctx := context.Background()

	signal := func(height uint64) {
		require.NoError(t, ingester.ProcessEndBlock(ctx, height, domain.BlockPoolMetadata{}))
		ingester.catchUp(ctx)
	}

	// A failed first fetch is retried, not skipped, when the next height arrives.
	fetcher.failAt[10] = true
	signal(10)
	require.Empty(t, fetcher.fetched)
	require.Equal(t, int64(0), repo.LastIngestedHeight())

	delete(fetcher.failAt, 10)
	signal(11)
	require.Equal(t, []int64{10, 11}, fetcher.fetched)

	signal(12)
	require.Equal(t, []int64{10, 11, 12}, fetcher.fetched)

	// A late signal for an older height changes nothing.
	signal(11)
	require.Equal(t, []int64{10, 11, 12}, fetcher.fetched)

	fetcher.failAt[14] = true
	signal(15)
	require.Equal(t, []int64{10, 11, 12, 13}, fetcher.fetched)
	require.Equal(t, int64(13), repo.LastIngestedHeight())

	delete(fetcher.failAt, 14)
	signal(15)
	require.Equal(t, []int64{10, 11, 12, 13, 14, 15}, fetcher.fetched)

	// 16-25 missed; only the last three are fetched.
	signal(25)
	require.Equal(t, []int64{10, 11, 12, 13, 14, 15, 23, 24, 25}, fetcher.fetched)
	require.Equal(t, int64(25), repo.LastIngestedHeight())

	// A height that keeps failing is skipped on its third attempt.
	fetcher.failAt[26] = true
	signal(26)
	signal(26)
	require.Equal(t, int64(25), repo.LastIngestedHeight())
	signal(27)
	require.Equal(t, []int64{10, 11, 12, 13, 14, 15, 23, 24, 25, 27}, fetcher.fetched)
	require.Equal(t, int64(27), repo.LastIngestedHeight())
}

// The startup backfill reads back to the node's oldest kept block (less the
// prune margin) in batches, ignoring the catch-up bound, values trades read far
// behind the chain at their time, and keeps the store hidden until it has caught up.
func TestIngesterBackfill(t *testing.T) {
	ctx := context.Background()
	pools := fakePools{1933: pool(1933, poolmanagertypes.CosmWasm, orderbookModel(uosmo, usdc))}
	pricing := fakePricing{uosmo: osmomath.MustNewBigDecFromStr("0.0251")}

	setup := func(fetcher *fakeFetcher, backfillBlocks int) (*Ingester, mvc.TradesRepository) {
		repo := repository.New(8 * 24 * time.Hour)
		return NewIngester(fetcher, pools, pricing, usdc, sixDecimals, nil, repo, 3, backfillBlocks, &log.NoOpLogger{}), repo
	}
	signal := func(ingester *Ingester, height uint64) {
		require.NoError(t, ingester.ProcessEndBlock(ctx, height, domain.BlockPoolMetadata{}))
	}
	// pass runs one catch-up pass if one is pending, as Run would.
	pass := func(ingester *Ingester) bool {
		select {
		case <-ingester.notify:
			ingester.catchUp(ctx)
			return true
		default:
			return false
		}
	}
	heights := func(from, to int64) []int64 {
		var out []int64
		for h := from; h <= to; h++ {
			out = append(out, h)
		}
		return out
	}

	t.Run("backfills to the node's earliest kept block", func(t *testing.T) {
		fetcher := &fakeFetcher{failAt: map[int64]bool{}, swaps: true, earliest: 1500}
		ingester, repo := setup(fetcher, 3000)

		// Asked for 3000 blocks before 5000, but the node keeps blocks from
		// 1500, so the backfill starts at 1500 plus the margin.
		signal(ingester, 5000)
		require.True(t, pass(ingester))
		require.Equal(t, heights(2500, 2999), fetcher.fetched, "one batch per pass")
		require.Zero(t, repo.LastIngestedHeight(), "hidden while backfilling")

		// The chain moves on during the backfill; the catch-up bound of 3 does not skip it.
		signal(ingester, 5001)
		signal(ingester, 5002)
		for pass(ingester) {
		}
		require.Equal(t, heights(2500, 5002), fetcher.fetched)
		require.Equal(t, int64(5002), repo.LastIngestedHeight())

		// Every trade is valued: within the catch-up bound of the chain at the
		// spot price, older ones from their USDC leg.
		poolID := uint64(1933)
		trades, _ := repo.ListTrades(domain.TradesFilter{PoolID: &poolID}, nil, 5)
		require.Len(t, trades, 5)
		for _, trade := range trades {
			require.NotNil(t, trade.UsdValueAtIngest, "height %d", trade.Key.BlockHeight)
			require.Equal(t, bigDec("0.025").String(), trade.UsdValueAtIngest.String())
		}
		require.Nil(t, ingester.history, "dropped once caught up")

		// Live again: the catch-up bound applies.
		signal(ingester, 5010)
		require.True(t, pass(ingester))
		require.Equal(t, heights(5008, 5010), fetcher.fetched[len(fetcher.fetched)-3:])
		require.Equal(t, int64(5010), repo.LastIngestedHeight())
	})

	t.Run("asks for fewer blocks than the node keeps", func(t *testing.T) {
		fetcher := &fakeFetcher{failAt: map[int64]bool{}, earliest: 1}
		ingester, _ := setup(fetcher, 100)
		signal(ingester, 5000)
		for pass(ingester) {
		}
		require.Equal(t, heights(4900, 5000), fetcher.fetched)
	})

	t.Run("forward-only without backfill or when the node keeps too little", func(t *testing.T) {
		for _, fetcher := range []*fakeFetcher{
			{failAt: map[int64]bool{}},
			{failAt: map[int64]bool{}, earliest: 4500},
		} {
			backfillBlocks := 3000
			if fetcher.earliest == 0 {
				backfillBlocks = 0
			}
			ingester, repo := setup(fetcher, backfillBlocks)
			signal(ingester, 5000)
			for pass(ingester) {
			}
			require.Equal(t, []int64{5000}, fetcher.fetched)
			require.Equal(t, int64(5000), repo.LastIngestedHeight())
		}
	})

	t.Run("a failed status read is retried, then the start is forward-only", func(t *testing.T) {
		// Recovers on the second signal and backfills from there.
		fetcher := &fakeFetcher{failAt: map[int64]bool{}, earliest: 1, earliestErr: errors.New("rpc unavailable")}
		ingester, repo := setup(fetcher, 100)
		signal(ingester, 5000)
		require.True(t, pass(ingester))
		require.Empty(t, fetcher.fetched)
		fetcher.earliestErr = nil
		signal(ingester, 5001)
		for pass(ingester) {
		}
		require.Equal(t, heights(4901, 5001), fetcher.fetched)
		require.Equal(t, int64(5001), repo.LastIngestedHeight())

		// Still failing on the third signal: forward-only from it.
		fetcher = &fakeFetcher{failAt: map[int64]bool{}, earliest: 1, earliestErr: errors.New("rpc unavailable")}
		ingester, repo = setup(fetcher, 100)
		for h := uint64(5000); h <= 5002; h++ {
			signal(ingester, h)
			for pass(ingester) {
			}
		}
		require.Equal(t, []int64{5002}, fetcher.fetched)
		require.Equal(t, int64(5002), repo.LastIngestedHeight())
	})

	t.Run("a node that cannot serve the old blocks ends the backfill and goes live", func(t *testing.T) {
		// Blocks before 4900 cannot be read, as on a node that keeps no old block results.
		fetcher := &fakeFetcher{failAt: map[int64]bool{}, earliest: 1}
		for h := int64(4000); h < 4900; h++ {
			fetcher.failAt[h] = true
		}
		ingester, repo := setup(fetcher, 1000)
		height := uint64(5000)
		signal(ingester, height)
		for signals := 0; repo.LastIngestedHeight() == 0; signals++ {
			require.Less(t, signals, 50, "the backfill should give up within a bounded number of signals")
			for pass(ingester) {
			}
			height++
			signal(ingester, height)
		}
		for pass(ingester) {
		}

		require.False(t, ingester.backfilling)
		require.Nil(t, ingester.history)
		require.Equal(t, int64(height), repo.LastIngestedHeight())
		// Only the first maxBackfillSkips old blocks were tried; live ingestion resumed within the catch-up bound.
		for _, h := range fetcher.fetched {
			require.GreaterOrEqual(t, h, int64(height)-3, "fetched %d", h)
		}
	})

	t.Run("a failed fetch during the backfill is retried on the next signal", func(t *testing.T) {
		fetcher := &fakeFetcher{failAt: map[int64]bool{4950: true}, earliest: 1}
		ingester, repo := setup(fetcher, 100)
		signal(ingester, 5000)
		for pass(ingester) {
		}
		require.Equal(t, heights(4900, 4949), fetcher.fetched)

		delete(fetcher.failAt, 4950)
		signal(ingester, 5001)
		for pass(ingester) {
		}
		require.Equal(t, heights(4900, 5001), fetcher.fetched)
		require.Equal(t, int64(5001), repo.LastIngestedHeight())
	})

	t.Run("live trades stay priced when the chain moves past the bound during a pass", func(t *testing.T) {
		fetcher := &fakeFetcher{failAt: map[int64]bool{}, swaps: true}
		ingester, repo := setup(fetcher, 0)
		signal(ingester, 100)
		require.True(t, pass(ingester))

		// Heights 101-103 are read in one pass; the chain reaches 110 while 101 is fetched.
		fetcher.onFetch = func(height int64) {
			if height == 101 {
				signal(ingester, 110)
			}
		}
		require.NoError(t, ingester.ProcessEndBlock(ctx, 103, domain.BlockPoolMetadata{}))
		ingester.catchUp(ctx)
		fetcher.onFetch = nil

		poolID := uint64(1933)
		trades, _ := repo.ListTrades(domain.TradesFilter{PoolID: &poolID}, nil, 10)
		require.Len(t, trades, 4)
		for _, trade := range trades {
			require.NotNil(t, trade.UsdValueAtIngest, "height %d", trade.Key.BlockHeight)
		}
	})
}
