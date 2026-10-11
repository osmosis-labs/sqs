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
	"github.com/osmosis-labs/osmosis/v28/ingest/types/cosmwasmpool"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v28/x/poolmanager/types"
	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/sqs/domain"
	"github.com/osmosis-labs/sqs/domain/cache"
	"github.com/osmosis-labs/sqs/domain/mocks"
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
	trades, err := ingester.extractTrades(context.Background(), block)
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
			trade, skip, err := n.normalise(context.Background(), tc.event, domain.TradeKey{BlockHeight: 1}, time.Unix(0, 0), "TX", make(priceCache))
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

type fakeFetcher struct {
	failAt  map[int64]bool
	fetched []int64
}

func (f *fakeFetcher) FetchBlock(_ context.Context, height int64) (BlockData, error) {
	if f.failAt[height] {
		return BlockData{}, errors.New("rpc unavailable")
	}
	f.fetched = append(f.fetched, height)
	return BlockData{Height: height, Time: time.Now()}, nil
}

// Heights are processed in order, forward-only from the first signal, failed
// heights are retried from where they stopped, and a lag beyond the catch-up
// bound skips the oldest blocks.
func TestIngesterCatchUp(t *testing.T) {
	fetcher := &fakeFetcher{failAt: map[int64]bool{}}
	repo := repository.New(8 * 24 * time.Hour)
	ingester := NewIngester(fetcher, fakePools{}, fakePricing{}, usdc, sixDecimals, nil, repo, 3, &log.NoOpLogger{})
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
}
