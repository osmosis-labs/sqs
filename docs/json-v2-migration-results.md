# JSON v2 migration: performance results

Companion to [json-v2-migration.md](json-v2-migration.md). The first part covers decoding of ingest models; [Encoding: quote responses](#encoding-quote-responses) covers the response serializer. Measured on an Apple M1 Pro with go1.27.3, fixture `router/usecase/routertesting/parsing/pools.json`, `-count 10`, compared with benchstat, with no other benchmarks running at the same time.

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

Quote serialization is unchanged by this decoding work: 27.6 µs → 25.2 µs (−8.9%) and 26.9 kB → 9.0 kB per response, compared with Echo's default serializer. The numeric encoding option below changes that.

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

## Encoding: quote responses

### Finding

A CPU profile of `BenchmarkQuoteResponseSerialization` on the plain v2 serializer showed the numeric methods dominating. Cumulative shares overlap:

- `LegacyDec.MarshalJSON`: 22%
- `Int.MarshalJSON`: 22%
- nested `encoding/json.Marshal`: 26%
- method dispatch, including children: 59%

The decimal method formats the value with `String`; the integer method uses `big.Int.MarshalText`, which produces the same decimal text as `String`. Both quote the text with a nested `json.Marshal` into a new slice. The outer v2 encoder then scans and copies that slice again.

### Change

`domain/osmomath.JSONMarshalOptions` registers a `json.MarshalToFunc` for `LegacyDec` and for `Int`. For a non-nil value, the function writes `v.String()` as a single string token to the encoder. It does no formatting of its own. For a nil-backed value (the zero struct), it returns `errors.ErrUnsupported` before writing anything. The type's own `MarshalJSON` then encodes it, so the dependencies keep ownership of that representation (currently `"0"`).

`delivery/http.JSONSerializer` passes the option on every response, with `jsontext.WithIndent` joined in for indented responses. Buffering, the trailing newline and encoding-failure atomicity are unchanged. Request decoding, SSE events (`WriteEvent`) and all other `json.Marshal` call sites keep plain v2 defaults.

`BigDec` is not registered. Quote responses carry none, and the endpoints that do (token prices) have not been measured.

### Results

`BenchmarkQuoteResponseSerialization` uses a prepared quote with 4 routes of 3 hops each (about 4.3 kB of JSON), run with `-count 10`. It compares three variants:

- `v1-echo-default`: Echo's default serializer, the original production codec
- `v2-plain`: the previous v2 serializer, without the option
- `v2`: production

| Comparison | time/op | B/op | allocs/op |
|---|---|---|---|
| previous v2 (HEAD before change) → v2 | 25.6 µs → 19.0 µs (−25.8%) | 8.99 kB → 5.00 kB (−44%) | 416 → 220 (−47%) |
| `v2-plain` → `v2` (same run) | 26.1 µs → 19.0 µs (−27.2%) | 8.99 kB → 5.00 kB (−44%) | 416 → 220 (−47%) |
| `v1-echo-default` → `v2` (same run) | 28.8 µs → 19.0 µs (−34.0%) | 26.9 kB → 5.0 kB (−81%) | 426 → 220 (−48%) |

All deltas have p < 0.001 (benchstat, n = 10 + 10). The control variant `v1-echo-default` did not move between the two runs (p = 0.08).

This measures serializing an already-built quote. It excludes routing, handler work and network I/O, so these figures do not establish the end-to-end latency improvement. That has not been measured.

After the change, the largest remaining costs are the owners' own formatting (`big.Int.String`, about 15% cumulative) and `sdk.Coins.MarshalJSON` (about 5%), which still uses its nested encoder.

### Behaviour and tests

The output is byte-identical to the methods.

- `TestJSONMarshalOptions_MatchesMethod` (`domain/osmomath/json_test.go`) compares output with and without the option, compact and indented. It covers these values:
  - nil-backed and initialized zero
  - ±1 and ±10⁻¹⁸
  - `int64` limits
  - the 256-bit `Int` and `LegacyDec` limits

  Each value appears in every position response models use:
  - value and pointer fields, including nil pointers
  - `omitempty` and `omitzero`
  - slices and maps
  - `any`

  The test also checks that encoding does not modify the input.
- `TestJSONMarshalOptions_PinnedValues` pins the representations: nil `LegacyDec` and nil `Int` are `"0"`, and an initialized zero `LegacyDec` is `"0.000000000000000000"`.
- `TestQuoteResponseSerialization_MatchesPlainV2` compares the benchmark quote, compact and indented.
- The serializer tests cover wiring, indentation, the trailing newline, and a failure after numeric output, which still leaves the response uncommitted.
- Handler tests for other endpoints run through `NewEcho` unchanged.
- A shared-option concurrency test runs under `-race`.

### Limitations

- `omitzero` on a nil-backed `LegacyDec` or `Int` panics in `IsZero`, with or without the option. This is unchanged dependency behaviour.
- The option reaches only values encoded by the encoder it is passed to. A type whose `MarshalJSON` calls `json.Marshal` internally, such as `sdk.Coins`, still encodes its contents through the dependencies' methods.
- If a dependency changes `String` or `MarshalJSON` so that they diverge, the differential tests fail. If a dependency later implements `MarshalJSONTo`, remove its registration.

## Validation

The implementation passed the repository build, `make test-unit`, focused race checks on the numeric adapter, parsing and ingest packages, and golangci-lint v2.14.0. Differential fuzzing ran for 90 seconds without divergence.

Integration added explicit escaped-string and exact overflow-boundary cases, clarified error-mutation behavior, and passed the full suite with coverage (`go test -mod=readonly ./... -coverprofile=... -covermode=count`). CI-version lint and `git diff --check` also passed after integration. The installed older linter cannot load this Go version; validation used the separately built CI version.

The encoding option passed:

- `go build -mod=readonly ./...`
- `make test-unit`
- the full suite with coverage; the new functions are 100% covered
- `-race` on the numeric, serializer, router use case and HTTP delivery packages
- golangci-lint v2.14.0, gofmt and `git diff --check`
