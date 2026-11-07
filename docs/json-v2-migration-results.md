# JSON v2 migration: decoding performance results

Companion to [json-v2-migration.md](json-v2-migration.md). Measured on an Apple M1 Pro with go1.27.3, fixture `router/usecase/routertesting/parsing/pools.json`, `-count 10`, compared with benchstat, with no other benchmarks running at the same time.

## Finding

Plain `encoding/json/v2` decoded the ingest models 7–8% slower than the previous Osmosis jsoniter wrapper. Allocation was lower. Profiles of each path, taken separately, showed substantial time in numeric `UnmarshalJSON` methods. The outer decoder scans each JSON value before handing its bytes to the method. The method then calls a nested `json.Unmarshal` to extract the string, scanning it again, before parsing the number. Tick and orderbook models hold thousands of these values.

## Change

`domain/osmomath.JSONUnmarshalOptions` registers a `json.UnmarshalFromFunc` for each of `LegacyDec`, `Int` and `BigDec`. For a JSON string, the function reads one token and calls the owning package's exported constructor:

- `LegacyNewDecFromStr`
- `NewIntFromString`
- `NewBigDecFromStr`

For any other kind, it returns `errors.ErrUnsupported` before consuming input, so the type's own `UnmarshalJSON` still handles `null`, numbers, objects and arrays.

The option is used only at these boundaries:

- `ingestUseCase.parsePool`, for the SQS and tick models
- the `routertesting/parsing` state-file readers

Echo request decoding and all other call sites keep plain v2 defaults.

## Results (ingest models and state file)

| Benchmark | wrapper | v2 | v2 + options | vs wrapper |
|---|---|---|---|---|
| All ingest models (per-pool payloads) | 36.7 ms | 39.7 ms | 34.2 ms | −6.9% |
| Pool state file | 39.3 ms | 41.8 ms | 36.6 ms | −7.0% |
| Largest tick model | 19.9 ms | 21.1 ms | 18.3 ms | −7.9% |
| Largest SQS model (orderbook) | 29.6 µs | 30.8 µs | 26.7 µs | −9.7% |
| Median SQS model | 1.89 µs | 2.13 µs | 1.83 µs | −3.4% |
| Median tick model | 2.33 µs | 2.62 µs | 2.40 µs | **+3.0%** |

Compared with the wrapper, allocated bytes fall by 25–50% and allocation counts by 32–47%.

Quote serialization is unchanged by this work: 27.6 µs → 25.2 µs (−8.9%) and 26.9 kB → 9.0 kB per response, compared with Echo's default serializer.

### Remaining tradeoff

On the median tick model (a handful of ticks, about 350 bytes), the remaining gap is 3% (about 70 ns). Profiles suggest fixed per-call and field-name processing costs contribute, but do not isolate their exact share of the difference. Further approaches considered included:

- consuming non-string values ourselves, which would restate the dependencies' `null` and wrong-kind behaviour
- copying the decimal parsers to work on bytes
- unsafe or buffer-lifetime tricks

These approaches were rejected on maintainability grounds. The aggregate over the committed fixture is 7% faster. This is not an end-to-end block-ingestion measurement: production results depend on the mix of updated pools and ticks, and on work outside JSON decoding. Not every payload is faster.

## Behaviour and compatibility

The differential tests are in `domain/osmomath/json_test.go`. The oracle is the dependency's own `UnmarshalJSON`. The tests cover:

- valid inputs and precision and range limits
- the dependencies' parser quirks: `+1.5`, `--1`, and base-0 `Int` forms such as `010`, which decodes to 8
- escaped strings
- `null` and wrong JSON kinds
- prepopulated targets and pointer, slice and map positions
- v2 strictness (duplicate names, invalid UTF-8, trailing data)
- concurrent use

A fuzz target compares both paths on arbitrary values. The fixture tests in `codec_compat_test.go` and `ingest/usecase/parse_pool_test.go` check that the optimized path matches the Osmosis wrapper.

Deliberate differences:

- A failed string-to-number parse leaves that numeric target unchanged. Fallback methods and the surrounding document decode can still partially mutate targets before an error. Callers discard failed decodes.
- `Int` parse errors carry an SQS message, because `NewIntFromString` reports failure only as a bool.
- `Int` decoding assigns a fresh value. The old method overwrote the `big.Int` in place, which also changed any copy of the target that shared it.

Limits of the option:

- It reaches only values decoded by the decoder it is passed to. Types whose `UnmarshalJSON` calls `json.Unmarshal` internally decode their contents through the dependencies' methods. These include `ingesttypes.TakerFeeMap`, the balancer chain model (`balancer.Pool`) and `passthroughdomain.PoolAPR` and `PoolFee`. Their results are correct but not faster.
- Chain models from the node go through the protobuf interface codec and are untouched.

## Maintenance

The adapter parses nothing itself; the dependencies own grammar, precision, range and error values.

- If a dependency upgrade changes a constructor, the adapter follows automatically.
- If an upgrade makes a constructor diverge from its `UnmarshalJSON`, the differential and fuzz tests fail with a concrete input.
- If a dependency later implements `UnmarshalJSONFrom`, remove its registration, or the whole adapter.

## Validation

The implementation passed the repository build, `make test-unit`, focused race checks on the numeric adapter, parsing and ingest packages, and golangci-lint v2.14.0. Differential fuzzing ran for 90 seconds without divergence.

Integration added explicit escaped-string and exact overflow-boundary cases, clarified error-mutation behavior, and passed the full suite with coverage (`go test -mod=readonly ./... -coverprofile=... -covermode=count`). CI-version lint and `git diff --check` also passed after integration. The installed older linter cannot load this Go version; validation used the separately built CI version.
