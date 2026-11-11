package parsing_test

import (
	"cmp"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/osmosis-labs/osmosis/osmomath"

	osmojson "github.com/osmosis-labs/osmosis/v31/ingest/types/json"
	"github.com/osmosis-labs/sqs/domain"
	sqsosmomath "github.com/osmosis-labs/sqs/domain/osmomath"
	ingesttypes "github.com/osmosis-labs/sqs/ingest/types"
	"github.com/osmosis-labs/sqs/router/usecase/routertesting/parsing"

	concentratedmodel "github.com/osmosis-labs/osmosis/v31/x/concentrated-liquidity/model"
	cosmwasmpoolmodel "github.com/osmosis-labs/osmosis/v31/x/cosmwasmpool/model"
	"github.com/osmosis-labs/osmosis/v31/x/gamm/pool-models/balancer"
	"github.com/osmosis-labs/osmosis/v31/x/gamm/pool-models/stableswap"
	poolmanagertypes "github.com/osmosis-labs/osmosis/v31/x/poolmanager/types"
)

// wrapperSerializedPool mirrors parsing.SerializedPool for the Osmosis jsoniter wrapper, which
// decodes a jsontext.Value field as base64 bytes rather than raw JSON.
type wrapperSerializedPool struct {
	Type      poolmanagertypes.PoolType `json:"type"`
	ChainPool osmojson.RawMessage       `json:"data"`
	SQSModel  ingesttypes.SQSPool       `json:"sqs_model"`
	TickModel *ingesttypes.TickModel    `json:"tick_model,omitempty"`
}

// requireSameMeaning fails unless both values re-encode identically with the Osmosis wrapper,
// which is how the Osmosis node writes these models. Comparing encodings rather than Go values
// avoids false mismatches from big.Int internals.
func requireSameMeaning(t testing.TB, want, got any) {
	t.Helper()
	wantBz, err := osmojson.Marshal(want)
	require.NoError(t, err)
	gotBz, err := osmojson.Marshal(got)
	require.NoError(t, err)
	require.Equal(t, string(wantBz), string(gotBz))
}

// requireSameV2Encoding compares deterministic v2 encodings. Used for v2 round trips, where a nil
// map or slice legitimately comes back empty: both encode as {} or [] and no SQS code tells them apart.
func requireSameV2Encoding(t testing.TB, want, got any) {
	t.Helper()
	wantBz, err := json.Marshal(want, json.Deterministic(true))
	require.NoError(t, err)
	gotBz, err := json.Marshal(got, json.Deterministic(true))
	require.NoError(t, err)
	require.Equal(t, string(wantBz), string(gotBz))
}

// mainnetPoolsFixture is the mainnet pool state fixture next to this test, written by the
// Osmosis jsoniter wrapper. Tests only read it.
const mainnetPoolsFixture = "pools.json"

func readFixture(t testing.TB, name string) []byte {
	t.Helper()
	bz, err := os.ReadFile(name)
	require.NoError(t, err)
	return bz
}

// TestPoolFixtureDecodesLikeOsmosisWrapper checks that encoding/json/v2 reads the mainnet pool
// fixture (written by the Osmosis jsoniter wrapper) into the same SQS models, tick models and
// chain pool models, so case-sensitive matching and the stricter grammar lose nothing.
func TestPoolFixtureDecodesLikeOsmosisWrapper(t *testing.T) {
	bz := readFixture(t, mainnetPoolsFixture)

	var viaWrapper []wrapperSerializedPool
	require.NoError(t, osmojson.Unmarshal(bz, &viaWrapper))

	// Plain v2 dispatches to the numeric types' UnmarshalJSON methods; the SQS options stream
	// them through the owners' constructors. Both must match the wrapper.
	for name, opts := range map[string]json.Options{"v2": json.DefaultOptionsV2(), "v2-sqs-numeric": sqsosmomath.JSONUnmarshalOptions} {
		t.Run(name, func(t *testing.T) {
			requirePoolFixtureMatchesWrapper(t, bz, viaWrapper, opts)
		})
	}
}

func requirePoolFixtureMatchesWrapper(t *testing.T, bz []byte, viaWrapper []wrapperSerializedPool, opts json.Options) {
	var viaV2 []parsing.SerializedPool
	require.NoError(t, json.Unmarshal(bz, &viaV2, opts))
	require.Len(t, viaV2, len(viaWrapper))
	require.NotEmpty(t, viaV2)

	seenTypes := map[poolmanagertypes.PoolType]int{}
	for i := range viaWrapper {
		want, got := viaWrapper[i], viaV2[i]
		require.Equal(t, want.Type, got.Type)
		seenTypes[got.Type]++

		requireSameMeaning(t, want.SQSModel, got.SQSModel)
		requireSameMeaning(t, want.TickModel, got.TickModel)
		require.JSONEq(t, string(want.ChainPool), string(got.ChainPool))

		switch got.Type {
		case poolmanagertypes.Concentrated:
			requireSameChainPoolDecode[concentratedmodel.Pool](t, got.ChainPool, opts)
		case poolmanagertypes.CosmWasm:
			requireSameChainPoolDecode[cosmwasmpoolmodel.CosmWasmPool](t, got.ChainPool, opts)
		case poolmanagertypes.Stableswap:
			requireSameChainPoolDecode[stableswap.Pool](t, got.ChainPool, opts)
		case poolmanagertypes.Balancer:
			requireSameChainPoolDecode[balancer.Pool](t, got.ChainPool, opts)
		}
	}

	// The fixture must exercise every pool model, or this test proves less than it claims.
	for _, poolType := range []poolmanagertypes.PoolType{poolmanagertypes.Balancer, poolmanagertypes.Stableswap, poolmanagertypes.Concentrated, poolmanagertypes.CosmWasm} {
		require.Positive(t, seenTypes[poolType], "fixture has no pool of type %s", poolType)
	}
}

func requireSameChainPoolDecode[T any](t *testing.T, raw jsontext.Value, opts json.Options) {
	t.Helper()
	var viaWrapper, viaV2 T
	require.NoError(t, osmojson.Unmarshal(raw, &viaWrapper))
	require.NoError(t, json.Unmarshal(raw, &viaV2, opts))
	requireSameMeaning(t, &viaWrapper, &viaV2)
}

// TestIngestedSQSModelDecodesLikeOsmosisWrapper replays the node's side of the ingest wire
// contract: the node encodes SQS and tick models with the Osmosis wrapper and SQS decodes them
// with encoding/json/v2.
func TestIngestedSQSModelDecodesLikeOsmosisWrapper(t *testing.T) {
	var pools []wrapperSerializedPool
	require.NoError(t, osmojson.Unmarshal(readFixture(t, mainnetPoolsFixture), &pools))

	for name, opts := range map[string]json.Options{"v2": json.DefaultOptionsV2(), "v2-sqs-numeric": sqsosmomath.JSONUnmarshalOptions} {
		t.Run(name, func(t *testing.T) {
			for _, pool := range pools {
				sqsModelBz, err := osmojson.Marshal(pool.SQSModel)
				require.NoError(t, err)
				var sqsModel ingesttypes.SQSPool
				require.NoError(t, json.Unmarshal(sqsModelBz, &sqsModel, opts))
				requireSameMeaning(t, pool.SQSModel, sqsModel)

				if pool.TickModel == nil {
					continue
				}
				tickModelBz, err := osmojson.Marshal(pool.TickModel)
				require.NoError(t, err)
				var tickModel ingesttypes.TickModel
				require.NoError(t, json.Unmarshal(tickModelBz, &tickModel, opts))
				requireSameMeaning(t, pool.TickModel, &tickModel)
			}
		})
	}
}

// TestStateFixturesDecodeLikeOsmosisWrapper covers the remaining committed state files.
func TestStateFixturesDecodeLikeOsmosisWrapper(t *testing.T) {
	t.Run("tokens", func(t *testing.T) {
		requireSameFixtureDecode[map[string]domain.Token](t, "tokens.json")
	})
	t.Run("pool denom metadata", func(t *testing.T) {
		requireSameFixtureDecode[domain.PoolDenomMetaDataMap](t, "pool_denom_metadata.json")
	})
	t.Run("candidate route search data", func(t *testing.T) {
		viaV2, err := parsing.ReadCandidateRouteSearchData("candidate_route_search_data.json")
		require.NoError(t, err)
		require.NotEmpty(t, viaV2)

		var serialized []struct {
			Denom      string                `json:"denom"`
			Pool       []osmojson.RawMessage `json:"pool"`
			Orderbooks []struct {
				PairDenom string              `json:"pair_denom"`
				Orderbook osmojson.RawMessage `json:"orderbook"`
			} `json:"orderbooks"`
		}
		require.NoError(t, osmojson.Unmarshal(readFixture(t, "candidate_route_search_data.json"), &serialized))
		require.Len(t, viaV2, len(serialized))
		for _, entry := range serialized {
			got := viaV2[entry.Denom]
			require.NotNil(t, got)
			require.Len(t, got.SortedPools, len(entry.Pool))
			for i, raw := range entry.Pool {
				var want domain.CandidatePoolWrapper
				require.NoError(t, osmojson.Unmarshal(raw, &want))
				requireSameMeaning(t, want, got.SortedPools[i])
			}
			for _, orderbook := range entry.Orderbooks {
				var want domain.CandidatePoolWrapper
				require.NoError(t, osmojson.Unmarshal(orderbook.Orderbook, &want))
				requireSameMeaning(t, want, got.CanonicalOrderbooks[orderbook.PairDenom])
			}
		}
	})
	t.Run("taker fees", func(t *testing.T) {
		viaV2, err := parsing.ReadTakerFees("taker_fees.json")
		require.NoError(t, err)
		viaWrapper := ingesttypes.TakerFeeMap{}
		require.NoError(t, osmojson.Unmarshal(readFixture(t, "taker_fees.json"), &viaWrapper))
		require.NotEmpty(t, viaV2)
		requireSameMeaning(t, viaWrapper, viaV2)
	})
}

func requireSameFixtureDecode[T any](t *testing.T, name string) {
	t.Helper()
	bz := readFixture(t, name)
	var viaWrapper, viaV2 T
	require.NoError(t, osmojson.Unmarshal(bz, &viaWrapper))
	require.NoError(t, json.Unmarshal(bz, &viaV2))
	requireSameMeaning(t, viaWrapper, viaV2)
}

// TestPoolStateRoundTrip exports the pool fixture with StorePools and imports it again with
// ReadPools. The export uses v2 encoding, so this also checks that v2 output (zero numbers and
// false booleans no longer omitted, [] for nil slices) imports to the same pools.
func TestPoolStateRoundTrip(t *testing.T) {
	pools, tickModels, err := parsing.ReadPools(mainnetPoolsFixture)
	require.NoError(t, err)

	exported := filepath.Join(t.TempDir(), "pools.json")
	require.NoError(t, parsing.StorePools(pools, tickModels, exported))

	reimported, reimportedTickModels, err := parsing.ReadPools(exported)
	require.NoError(t, err)
	require.Len(t, reimported, len(pools))
	require.Len(t, reimportedTickModels, len(tickModels))

	for i := range pools {
		require.Equal(t, pools[i].GetType(), reimported[i].GetType())
		requireSameV2Encoding(t, pools[i].GetUnderlyingPool(), reimported[i].GetUnderlyingPool())
		requireSameV2Encoding(t, pools[i].GetSQSPoolModel(), reimported[i].GetSQSPoolModel())
	}
	for id, tickModel := range tickModels {
		requireSameV2Encoding(t, tickModel, reimportedTickModels[id])
	}

	// A second export of the reimported state is byte-identical: state files are deterministic.
	exportedAgain := filepath.Join(t.TempDir(), "pools.json")
	require.NoError(t, parsing.StorePools(reimported, reimportedTickModels, exportedAgain))
	require.Equal(t, readFixture(t, exported), readFixture(t, exportedAgain))
}

// TestIngestStateRoundTrip covers the numeric (uint64) map keys of the denom liquidity map.
func TestIngestStateRoundTrip(t *testing.T) {
	pools, _, err := parsing.ReadPools(mainnetPoolsFixture)
	require.NoError(t, err)

	liquidity := domain.DenomPoolLiquidityMap{}
	for _, pool := range pools {
		for _, balance := range pool.GetSQSPoolModel().Balances {
			data := liquidity[balance.Denom]
			if data.Pools == nil {
				data.Pools = map[uint64]osmomath.Int{}
				data.TotalLiquidity = osmomath.ZeroInt()
			}
			data.Pools[pool.GetId()] = balance.Amount
			data.TotalLiquidity = data.TotalLiquidity.Add(balance.Amount)
			data.EffectiveTotalLiquidity = osmomath.BigDecFromSDKInt(data.TotalLiquidity)
			liquidity[balance.Denom] = data
		}
	}
	require.NotEmpty(t, liquidity)

	exported := filepath.Join(t.TempDir(), "ingest.json")
	require.NoError(t, parsing.StoreIngest(liquidity, exported))
	reimported, err := parsing.LoadIngest(exported)
	require.NoError(t, err)
	requireSameV2Encoding(t, liquidity, reimported)
}

func TestCandidateRouteExportOrdering(t *testing.T) {
	state := map[string]*domain.CandidateRouteDenomData{}
	for _, denom := range []string{"uosmo", "uatom", "uion"} {
		state[denom] = &domain.CandidateRouteDenomData{
			SortedPools: []domain.CandidatePoolWrapper{{ID: 9}, {ID: 1}},
			CanonicalOrderbooks: map[string]domain.CandidatePoolWrapper{
				"uusdc": {ID: 3},
				"ubtc":  {ID: 2},
			},
		}
	}

	var firstExport []byte
	for range 5 {
		path := filepath.Join(t.TempDir(), "candidate_routes.json")
		require.NoError(t, parsing.StoreCandidateRouteSearchData(state, path))
		data := readFixture(t, path)
		if firstExport == nil {
			firstExport = data
		}
		require.Equal(t, firstExport, data)

		var entries []struct {
			Denom      string                        `json:"denom"`
			Pools      []domain.CandidatePoolWrapper `json:"pool"`
			Orderbooks []struct {
				PairDenom string `json:"pair_denom"`
			} `json:"orderbooks"`
		}
		require.NoError(t, json.Unmarshal(data, &entries))
		require.Len(t, entries, 3)
		for i, denom := range []string{"uatom", "uion", "uosmo"} {
			require.Equal(t, denom, entries[i].Denom)
			require.Len(t, entries[i].Orderbooks, 2)
			require.Equal(t, "ubtc", entries[i].Orderbooks[0].PairDenom)
			require.Equal(t, "uusdc", entries[i].Orderbooks[1].PairDenom)
			// Routing priority is meaningful; only map-derived arrays are sorted.
			require.Len(t, entries[i].Pools, 2)
			require.Equal(t, uint64(9), entries[i].Pools[0].ID)
			require.Equal(t, uint64(1), entries[i].Pools[1].ID)
		}
		restored, err := parsing.ReadCandidateRouteSearchData(path)
		require.NoError(t, err)
		requireSameV2Encoding(t, state, restored)
	}
}

// ingestBenchmarkCodecs are the decoders compared by the ingest benchmarks: the Osmosis jsoniter
// wrapper (the previous codec), plain encoding/json/v2, and v2 with the SQS numeric options that
// production ingest and state reads use.
var ingestBenchmarkCodecs = []struct {
	name      string
	unmarshal func([]byte, any) error
}{
	{name: "osmosis-wrapper", unmarshal: osmojson.Unmarshal},
	{name: "v2", unmarshal: func(bz []byte, out any) error { return json.Unmarshal(bz, out) }},
	{name: "v2-sqs-numeric", unmarshal: func(bz []byte, out any) error {
		return json.Unmarshal(bz, out, sqsosmomath.JSONUnmarshalOptions)
	}},
}

// ingestBenchmarkPayloads returns the node-encoded SQS and tick models of every fixture pool.
func ingestBenchmarkPayloads(b *testing.B) (sqsModels, tickModels [][]byte) {
	var pools []wrapperSerializedPool
	require.NoError(b, osmojson.Unmarshal(readFixture(b, mainnetPoolsFixture), &pools))
	for _, pool := range pools {
		bz, err := osmojson.Marshal(pool.SQSModel)
		require.NoError(b, err)
		sqsModels = append(sqsModels, bz)
		if pool.TickModel != nil {
			bz, err := osmojson.Marshal(pool.TickModel)
			require.NoError(b, err)
			tickModels = append(tickModels, bz)
		}
	}
	return sqsModels, tickModels
}

// BenchmarkIngestModelDecoding decodes the per-pool SQS and tick model payloads of the ingest
// path, built from every pool in the mainnet fixture.
func BenchmarkIngestModelDecoding(b *testing.B) {
	sqsModels, tickModels := ingestBenchmarkPayloads(b)
	totalBytes := 0
	for _, bz := range slices.Concat(sqsModels, tickModels) {
		totalBytes += len(bz)
	}

	for _, bc := range ingestBenchmarkCodecs {
		b.Run(bc.name, func(b *testing.B) {
			b.SetBytes(int64(totalBytes))
			b.ReportAllocs()
			for b.Loop() {
				for _, bz := range sqsModels {
					var model ingesttypes.SQSPool
					if err := bc.unmarshal(bz, &model); err != nil {
						b.Fatal(err)
					}
				}
				for _, bz := range tickModels {
					var model ingesttypes.TickModel
					if err := bc.unmarshal(bz, &model); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}

// BenchmarkIngestModelDecodingBySize decodes single payloads of representative sizes, so a gain
// on the aggregate is not only a gain on the few largest tick models.
func BenchmarkIngestModelDecodingBySize(b *testing.B) {
	sqsModels, tickModels := ingestBenchmarkPayloads(b)
	bySize := func(a, b []byte) int { return cmp.Compare(len(a), len(b)) }
	slices.SortFunc(sqsModels, bySize)
	slices.SortFunc(tickModels, bySize)

	payloads := []struct {
		name     string
		bz       []byte
		newModel func() any
	}{
		{name: "sqs-model-median", bz: sqsModels[len(sqsModels)/2], newModel: func() any { return new(ingesttypes.SQSPool) }},
		{name: "sqs-model-largest", bz: sqsModels[len(sqsModels)-1], newModel: func() any { return new(ingesttypes.SQSPool) }},
		{name: "tick-model-median", bz: tickModels[len(tickModels)/2], newModel: func() any { return new(ingesttypes.TickModel) }},
		{name: "tick-model-largest", bz: tickModels[len(tickModels)-1], newModel: func() any { return new(ingesttypes.TickModel) }},
	}
	for _, payload := range payloads {
		for _, bc := range ingestBenchmarkCodecs {
			b.Run(payload.name+"/"+bc.name, func(b *testing.B) {
				b.SetBytes(int64(len(payload.bz)))
				b.ReportAllocs()
				for b.Loop() {
					if err := bc.unmarshal(payload.bz, payload.newModel()); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

// BenchmarkPoolStateFileDecoding compares the codecs on the full pool state file.
func BenchmarkPoolStateFileDecoding(b *testing.B) {
	bz := readFixture(b, mainnetPoolsFixture)
	b.Run("osmosis-wrapper", func(b *testing.B) {
		b.SetBytes(int64(len(bz)))
		b.ReportAllocs()
		for b.Loop() {
			var pools []wrapperSerializedPool
			if err := osmojson.Unmarshal(bz, &pools); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("v2", func(b *testing.B) {
		b.SetBytes(int64(len(bz)))
		b.ReportAllocs()
		for b.Loop() {
			var pools []parsing.SerializedPool
			if err := json.Unmarshal(bz, &pools); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("v2-sqs-numeric", func(b *testing.B) {
		b.SetBytes(int64(len(bz)))
		b.ReportAllocs()
		for b.Loop() {
			var pools []parsing.SerializedPool
			if err := json.Unmarshal(bz, &pools, sqsosmomath.JSONUnmarshalOptions); err != nil {
				b.Fatal(err)
			}
		}
	})
}
