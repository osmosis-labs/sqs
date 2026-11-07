# JSON v2 migration plan

Migrate SQS-owned JSON processing to Go's `encoding/json/v2` and `encoding/json/jsontext`. Marko has explicitly authorized public API changes, including JSON representation and decoding behavior. Use v2 defaults unless a chain protocol or application invariant requires a specific exception.

The repository targets Go 1.27.2. The existing `encoding/json` API already uses the v2 engine on Go 1.27, so this migration targets consistent semantics, stricter parsing, and streaming APIs. Performance improvements must be measured against the current toolchain.

## Scope and decisions

- Migrate direct standard-library JSON usage in application code and relevant tests, including HTTP responses, SSE payloads, external response decoding, order-cache decoding, and claim-message construction.
- Install a v2 serializer on the application's Echo instance. Updating direct imports alone does not change `c.JSON` or Echo request binding.
- Adopt case-sensitive field matching, duplicate-name and invalid-UTF-8 rejection, v2 empty-collection representation, and v2 `omitempty` semantics. Zero numeric and boolean fields may become visible. Use `omitzero` only where omission expresses the intended application contract, with a test explaining that choice.
- Preserve numerical precision and the intended string representation of Cosmos/Osmosis integer and decimal values. Public API flexibility does not authorize changing financial values or chain message semantics.
- Preserve Cosmos protobuf/interface codecs such as `UnmarshalInterfaceJSON`. Do not replace protobuf JSON with generic struct decoding.
- Inspect the pinned Osmosis JSON wrapper before replacing it. Migrate compatible SQS-owned wrapper call sites; retain and document any adapter required for chain interoperability. Do not edit dependencies or generated protobuf code.
- Do not add blanket v1 compatibility options or a fallback that silently retries rejected input with v1. Keep any remaining v1 dependency narrow and documented.
- Keep the existing Go version and dependency versions unless a demonstrated migration requirement demands a change. Leave commit, push, and PR creation to a separate request.

## 1. Establish the baseline and codec inventory

Inspect local agent instructions, worktree status, the actual Go toolchain, and the pinned dependency source. Preserve unrelated work. Inventory direct imports, aliased JSON imports, custom marshal methods, Echo serializer use, and JSON-backed fixtures.

Key locations:

- `app/sidecar_query_server.go`: Echo construction.
- `delivery/http/event.go`: SSE encoding and framing.
- `sqsutil/sqshttp/sqs_http.go`: generic external HTTP response decoding.
- `chainregistry/usecase/chainregistry_usecase.go` and `tokens/usecase/token_registry.go`: registry inputs.
- `tokens/usecase/pricing/coingeckopoller/fetch.go`: bounded response decoding and exact decimal conversion behavior.
- `domain/sync/atomic/int.go`: custom JSON methods.
- `ingest/usecase/plugins/orderbook/orderscache/plugin.go` and `claimbot/tx.go`: order state and chain-message JSON.
- `ingest/usecase/ingest_usecase.go`, `domain/cosmwasm/cosmwasm_query.go`, and `router/usecase/routertesting/parsing/mainnet_pools.go`: Osmosis wrapper boundaries.
- `router/usecase/quote_in_given_out.go`, `quote_out_given_in.go`, and `domain/tx_fee_info.go`: exposed response types.

Run relevant existing tests before edits where feasible and record baseline failures. Inspect the wrapper implementation and dependency custom marshalers; their behavior was not established during the initial assessment.

## 2. Migrate application JSON and Echo

Use v2 `Marshal` and `Unmarshal` for byte slices, `MarshalWrite` and `UnmarshalRead` for single JSON values on writers/readers, and `jsontext.Value` where raw JSON is needed. Use the actual Go 1.27 APIs rather than earlier experimental examples. Retain the conventional `MarshalJSON` and `UnmarshalJSON` interfaces where dependency interoperability requires them; add streaming methods only when useful and verified.

Implement an Echo `JSONSerializer` in the HTTP delivery package and wire it into production construction. Support indentation and request decoding, map syntactic/semantic decode failures to suitable HTTP client errors, and preserve meaningful non-JSON errors. Test the serializer through Echo's public interfaces, including binding and response status/content type. Account for partial response writes on encoding failure so handlers do not append an error response to an already committed JSON body.

Keep SSE event framing, flush behavior, and cancellation semantics intact while changing payload encoding. Avoid introducing unbounded buffering or emitting a partial SSE event on serialization failure.

Replace avoidable `io.ReadAll` plus decode sequences where readers are available. Preserve existing response limits, timeouts, and body closure. Where the migrated generic HTTP helper lacks a size limit, choose a documented limit based on its actual callers or make it caller-configurable; reject oversized input rather than accepting a truncated prefix. Decode exactly one value and reject trailing junk or a second document. Do not reject unknown fields globally without an application-specific reason.

## 3. Resolve Osmosis and Cosmos boundaries

Compare the pinned Osmosis wrapper with v2 using representative SQS pool and tick fixtures, CosmWasm query payloads, and custom integer/decimal types. Replace wrapper use where data meaning and chain interoperability are preserved. Keep the Cosmos interface registry codec for chain models and protobuf Any handling.

Audit pointer/value marshal dispatch, numeric map keys, raw messages, and zero/nil custom numeric values. Exercise state export/import round trips. Use deterministic output only where persisted artifacts or protocol consumers require stable bytes; JSON object member ordering is otherwise not an API guarantee.

Document each retained codec boundary with its actual reason. Do not claim every transitive dependency has migrated.

## 4. Update contracts and focused coverage

Update quote and other affected response fixtures to the chosen v2 behavior. Review schema annotations and generated API documentation where the externally described contract changes, using the repository's generation workflow rather than hand-editing generated Go files.

Add targeted coverage for:

- Echo response encoding and request decoding, including malformed JSON, duplicate names, invalid UTF-8, field-name case, and trailing input.
- Nil/empty collections, zero booleans/numbers, optional objects, and numeric custom marshalers in representative quote responses.
- External response size limits and existing Coingecko partial-answer/error behavior.
- Pool/tick ingestion, state round trips, and chain-message representation at migrated wrapper boundaries.
- SSE payload validity and framing after serialization changes.

Reuse existing fixture and suite infrastructure. Assert JSON meaning where object order is irrelevant; assert exact bytes only for an actual wire-format invariant. Do not mechanically rewrite tests to accept whatever the new encoder produces.

## 5. Validate and measure

Format changed Go files, run focused package tests during implementation, then run the repository checks:

```sh
go build -mod=readonly ./...
make test-unit
make lint
git diff --check
```

CI additionally runs the full suite with coverage via `go test -v ./... -coverprofile=coverage.out -covermode=count -json`; run equivalent coverage validation when feasible. Use the configured golangci-lint version (CI pins v2.14.0). Run focused race checks for changed concurrent paths when applicable. Report dependency, native-library, external-service, or tooling blockers explicitly; do not weaken checks to obtain a pass.

Add focused benchmarks for representative quote serialization and SQS pool/tick decoding. Compare the current codec against the migrated codec on the same Go 1.27 toolchain, fixture, and machine, recording allocations as well as runtime. Keep benchmark work bounded and avoid live-chain dependencies. Investigate material regressions; do not promise a speedup from an import change.

## Completion criteria

- Production Echo uses the v2 serializer, and SQS-owned JSON sites use v2 or have a documented protocol-specific exception.
- Public response changes are intentional and covered; chain payloads and financial values retain their intended meaning.
- Focused tests and required build/test/lint checks pass, or the exact verification blockers and baseline failures are reported.
- The implementation report lists remaining codec boundaries, observable API changes, benchmark findings, and verification gaps.
- Review the final diff for unrelated changes and leave implementation edits available for Marko to review.

## References

- [Go 1.27 release notes](https://go.dev/doc/go1.27#standard-library)
- [JSON migration semantics](https://pkg.go.dev/encoding/json#hdr-Migrating_to_v2)
- [Go JSON v2 overview](https://go.dev/blog/jsonv2)
