<!--
Guiding Principles:

Changelogs are for humans, not machines.
There should be an entry for every single version.
The same types of changes should be grouped.
Versions and sections should be linkable.
The latest version comes first.
The release date of each version is displayed.
Mention whether you follow Semantic Versioning.

Usage:

Change log entries are to be added to the Unreleased section under the
appropriate stanza (see below). Each entry should ideally include a tag and
the Github issue reference in the following format:

* (<tag>) \#<issue-number> message

The issue numbers will later be link-ified during the release process so you do
not have to worry about including a link manually, but you can if you wish.

Types of changes (Stanzas):

"Features" for new features.
"Improvements" for changes in existing functionality.
"Deprecated" for soon-to-be removed features.
"Bug Fixes" for any bug fixes.
"Client Breaking" for breaking CLI commands and REST routes used by end-users.
"API Breaking" for breaking exported APIs used by developers building on SDK.
"State Machine Breaking" for any changes that result in a different AppState
given same genesisState and txList.
Ref: https://keepachangelog.com/en/1.0.0/
-->

# Changelog

Unreleased

- Fixed claimbot panics due nil routerrepo.RouterRepository in txGasCalulator

## v27.2.0

- #593 - Add denoms filter for /pools endpoint

## v27.1.0

- #553 - Add pagination + sorting + query filtering support for /pools endpoint 

## v26.2.0

- #554 - [FIX] Add missing checks for transmuter's limiter 
- #548 - Return base fee in /quote regardless of simulation success.
- #547 - Add /quote simulation for "out given in" single routes.
- #526 - Refactor gas estimation APIs
- #524 - Claimbot

## v26.1.0

e42b32bc SQS-412 | Active Orders Query: SSE (#518)

## v26.0.1

0c663b7f SQS-417 | Fix orderbook order Quantity parsing (#519)

## v26.0.0

- ee288006 Uni-directional taker fee (#510)
- 2a657033 Fix orderbook ProcessPool panic (#498)

## v25.18.0

- 10b84b4c Fix sqsdomain package version (#508)
- ec649caa SQS-375 | Unit test createFormattedLimitOrder (#492)
- 142faa74 Fix: Orderbook ProcessPool panic (#497)
- f1efd83f feat: improve test stability by optional pricing source (#490)
- 4133a718 fix: rename tenderming to tendermint (#488)

## v25.17.0
- 4f5b8b81 chore: active orders observability TODOs (#469)

## v25.16.0

- Move active orders query to the passthrough use case

## v25.15.0

- Add query for active orders in the orderbook

## v25.14.0

- Fix bug withe duplicate metrics registration.

## v25.13.2

- Reduce metrics cardinality, removing labels for coingecko cache metrics.

## v25.13.1

- Fix `/config` query bug for chain handler use case returning the default that is overriden by CoinGecko pricing source.

## v25.13.0

- Reduce metrics cardinality, removing labels for most cases.
- Define sane config defaults, allowing the ability to override them via a config file or environment variables (backwards compatible).
- Remove unused `debug` config and flag.


## v25.12.0

- Add router and candidate route search options to disable route cache

## v25.11.0

- Fix concurrency bug in data fetcher prefetching.
- Fix error response status codes in /quote and /custom-direct-quote

## v25.10.0

- Orderbook plugin
- Change tick iteration exit condition in CL in orderbook from zero to smallest Dec (18 decimals).
- Add rounding direction to the order book
- Fix bug in skipping Numia API response errors
- Ingest tracing
- Fix `/tokens/metadata` query for multiple `chainDenom`
- Fix multi-hop /custom-direct-quote bug
- Apply taker fee and spread factor for fill bot amount estimation
- Reduce fill bot slippage bound by one ulp
- Fallback to filling each message individually.
- Active orders query

## v25.9.0

- Add staking rewards to the unclaimed rewards and total assets categories of the /portfolio-assets query
- Implement with_market_incentives query param for `/pools` that formats fees and APRs data on pools.

## v25.8.1

- Bump sqsdomain

## 25.8.0 (broken dependency)

- Remove sqs_requests_total and sqs_request_duration_seconds metrics
- Numia Pools APR fetcher, associated configs and wiring
- Timeseries pool fees fetcher, associated configs and wiring
- Fix alloyed overflow bug
- Alloyed rate limiter boilerplate in sqsdomain
- Add rate limiter to alloyed pools

Chain compatibility: v25.x-c775cee7-1722825184

## 25.7.0

- Fix bug with filters n /pools API

Chain compatibility: v25.x-c8140e68-1722532245

## 25.6.0

- Mitigate the alloy rebalancing router breakage by invalidating route cache if all routes fail and ordering denoms by balances.
- Prioritize canonical orderbook in router and caches
- Add liquidity filter to pools query
- Fix bug applying invalid pool liquidity filter in the liquidity pricing pipeline.
- In given out /router/quite refactor

Chain compatibility: v25.x-c8140e68-1722532245

## v25.5.0

- Switch quotes onto new candidate routes system

## v25.4.1

- Resolve goroutine leak stemming from creating a grpc connection for every cosmwasm pool in route. Share one connection.

## v25.4.0

- Remove spread factor from fee as it is included in the price impact
- Format contract address in canonical orderbook endpoints

## v25.3.2

- Fix telemetry issues around coingecko pricing source

## v25.3.1

- Fix goroutine leak in worker pool for prices

## v25.3.0

- Worker pool abstraction implementation & tests
- Switch prices to worker pool and remove concurrency for quotes since we support only one quote denom atm.
- Switch ingest block processing system to rely on worker pool with 2 block processing workers.
- Wait for cold-start (first block) to be processed before starting the next block to avoid overloading the system.
- Create a separate simple router usecase to be used in pricing and avoid mixing up configs and caches.
- [Q3] Auto-update asset list at regular intervals, added `update-assets-height-interval` config
- Switched to GRPC gateway from Tendermint RPC. `grpc-tendermint-rpc-endpoint` config for Tendermint RPC. `grpc-gateway-endpoint` for GRPC gateway.
- Added config for orderbook code id `pools.orderbook-code-ids`
- Move pricing logic to the dynamic min liquidity filter configuration
- Implement candidate route optimization for the pricing

Chain compatibility: `osmolabs/osmosis-dev:v25.x-49e8ee9`

## v25.2.0

- Fix bug with allUSDT swaps resulting in extreme price impact due to config and filtering issues.
- Fix bug of overapplying quote denom scaling factor in pool liquidity pricer.
- Pool liquidity capitalization
- Make alloyed transmuter pools receive more rank in routing

Chain compatibility: `osmolabs/osmosis-dev:v25.x-a1bf5551-1718937537`

## v25.1.1

- Add fault tolerance in candidate route conversion when pools are breaking.
- Hotfix for Alloyed pool swaps failing due to not having LP share in balances. 

## v25.1.0

- [SQS-Routing] API to force quote over a given route

## v25.0.0

- Create a config for pricing worker min liquidity capitalization and set it to 5 USD by default.
- [Dynamic Min Liquidity] new ingester methods for acquiring necessary metadata.
- bugfix: PoolWrapper Validate panic
- SQS: Fix /version endpoint and let it work in containers
- Alloyed pool support

**Note**: we jumped major versions from v19 to v26 to be compatible with the chain.

## v0.19.1

- Record errors with OpenTelemetry.

## v0.19.0

- Ingester data transformations for dynamic min liquidity.
- Optimize dynamic splits by caching repeated calls within callback.
- Remove queue from pricing worker
- Return single route quote if split quote fails
- CoinGecko pricing source support
- Flight recording of slow requests and dependency updates
- Consistently rename min liqudity filter across pool models & configs to min pool liquidity capitalization (cap)
- Validate token in and out in /router quote endpoints to ensure that they are not equal to each other & clean up tokens/prices
- Hack to fallback to precision of 18 when estimating spot price for Astroport pools via quotes.

## 0.18.5

- Asset list V2 integration

## 0.18.4

-   Reduce cardinality of duration metrics
-   Clean up chain pricing
-   Charge taker fee for transmuter pools

## v0.17.11 & 0.18.3

-   Fix Astroport PCL spot price bug - failure to utilize token out denom for quote estimate in edge cases
-   Fix pricing bug where we would incorrectly apply scaling factor to the price
    that is already correctly scaled when computing the price using the alternative (quote-based) method.

## v0.17.10

-   /config-private endpoint, mask OTEL config in /config endpoint

## v0.17.9

Fixes for pricing cache and min liquidity param

## v0.17.8

-   Rebuild image from new dockerfile

## v0.17.7

-   Custom sample rate config

## v0.17.5

-   Propagate --host CLI config for Sentry

## v0.17.4

-   Cache no candidate or ranked routes

## v0.17.3

-   Skip pool filtering if min osmo liquidity is zero

## v0.17.2

-   Fix bug with max split routes parameter

## v0.17.0

-   Pricing ingest worker
-   Remove support for unlisted tokens in the router and for prices
-   Healthcheck observes price updates
-   Never expire cache for USDC prices as they are computed in the background on every block where update occurred

## v0.16.0

-   Pricing options; pricing source wiring at the app level
-   Router options; remove GetOptimalQuoteFromConfig API.
-   Fetch only required taker fees instead of all.
-   Pre-allocate buffers in GetCandidateRoutes
-   Unsafe cast in GetCandidateRoutes for performance
-   Use slice instead of map in GetCandidateRoutes for performance
-   More performance tricks in GetCandidateRoutes
-   Cache zero routes for lower TTL if none found between token A and B
-   Validate chain denom parameters in /quote and /routes and /prices

## v0.15.0

-   Sentry CORS config
-   v24 import paths
-   Speedup for Order of Magnitude
-   Remove redundant allocations
-   LRU cache for tick to sqrt price

## v0.14.5

Add CORS header for Sentry

## v0.14.4

Nanosecond block process duration metric

## v0.14.3

Register ingest Prometheus metrics.

## v0.14.2

Expose port 50052 on Docker image

## v0.14.1

Sentry tracing config for /router/quote

## v0.14.0

ADR-006 stage 2 - GRPC Ingest Refactor

## v0.13.2

Revert Astroport spot price hot fix.
https://wallet.keplr.app/chains/osmosis/proposals/762

## v0.13.1

-   [#160](https://github.com/osmosis-labs/sqs/pull/160) Custom sampling rate per endpoint.

## v0.13.0

-   [#158](https://github.com/osmosis-labs/sqs/pull/158) Integrate Sentry & add new configs.

## v0.12.0

-   [#143](https://github.com/osmosis-labs/sqs/pull/143) light clean ups from the data ingest refactor.
-   [#148](https://github.com/osmosis-labs/sqs/pull/148) white whale switch of base and quote denoms in spot price.
-   [#147](https://github.com/osmosis-labs/sqs/pull/147) GRPC ingester config, code gen and wiring.

## v0.11.1

Attempt transmuter fix

## v0.11.0

Fix excessive price impact bug.

## v0.10.0

-   [#108](https://github.com/osmosis-labs/sqs/pull/107) Add code id (omitempty) to /quote route response
-   [#107](https://github.com/osmosis-labs/sqs/pull/107) Invert spot price in quotes. Break quote API

## v0.9.1

Hot fix for white whale base quote confusion

## v0.9.0

-   Support all asset list v1 tokens
-   Use spot price in pricing

## v0.8.4

Hot fix for astroport PCL base quote confusion

## v0.8.3

Fix for CW pools filtering

## v0.8.2

Do not error on spot price error in results, return zero instead.

## v0.8.1

Deprioritize non-transmuter pools

Fixes cosmwasm pools config issue where unsupported pools were getting into the router and breaking it.

## v0.7.3

Fixes cosmwasm pools config issue where unsupported pools were getting into the router and breaking it.

## v0.7.2

-   [#100](https://github.com/osmosis-labs/sqs/pull/100) Format in over out spot price in quotes.

## v0.7.0

-   [#99](https://github.com/osmosis-labs/sqs/pull/99) Move candidate routes cache out of Redis. Remove route overwrite

Various performance improvement optimizations.

## v0.6.0

-   [#85](https://github.com/osmosis-labs/sqs/pull/85) /tokens/prices endpoint

## v0.5.0

Human readable denom params in router

-   [#84](https://github.com/osmosis-labs/sqs/pull/84) enable swagger
-   [#83](https://github.com/osmosis-labs/sqs/pull/83) /tokens/metadata/:denom endpoint

## v0.4.0

-   [#81](https://github.com/osmosis-labs/osmosis/pull/81) Add support for single quotes as param in /quote.
-   Rename import paths to v23
-   Change Osmosis dependency to point to v23.x branch

## v0.3.0

## v0.2.2

## v0.2.1

-   [#76](https://github.com/osmosis-labs/osmosis/pull/76) Keep `/pools` endpoint only, allowing the IDs parameter

## v0.2.0

-   [#75](https://github.com/osmosis-labs/osmosis/pull/75) Break /pools/all and introduce /pools?IDs

## v0.1.3

-   [#52](https://github.com/osmosis-labs/osmosis/pull/52) Fix key separator issue breaking tokenfactory denoms.
-   [#53](https://github.com/osmosis-labs/osmosis/pull/53) Fix /version query
-   [#54](https://github.com/osmosis-labs/osmosis/pull/54) Return all paramters from /config (as opposed to just router config)
-   [#64](https://github.com/osmosis-labs/osmosis/pull/64) refactor: GetTakerFee by pool ID - avoid getting all taker fees

## v0.1.2

-   [#46](https://github.com/osmosis-labs/osmosis/pull/46) Various fixes around cosmwasm pool implementation after testing Astroport PCL on testnet.

## v0.1.1

-   [#45](https://github.com/osmosis-labs/osmosis/pull/45) Fix build issue by updating sqsdomain dep.
    Update config value.

## v0.1.0

-   [#43](https://github.com/osmosis-labs/osmosis/pull/43) Implement generalized CosmWasm pools into router that interact with the chain for computing quotes. Expose spot price by pool ID API.
