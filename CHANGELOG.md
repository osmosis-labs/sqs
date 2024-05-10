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

## v0.17.11

- Fix Astroport PCL spot price bug - failure to utilize token out denom for quote estimate in edge cases

## v0.17.10

- /config-private endpoint, mask OTEL config in /config endpoint

## v0.17.8

- Rebuild image from new dockerfile

## v0.17.7

- Custom sample rate config

## v0.17.5

- Propagate --host CLI config for Sentry

## v0.17.4

- Cache no candidate or ranked routes

## v0.17.3

- Skip pool filtering if min osmo liquidity is zero

## v0.17.2

- Fix bug with max split routes parameter

## v0.17.0

- Pricing ingest worker
- Remove support for unlisted tokens in the router and for prices
- Healthcheck observes price updates
- Never expire cache for USDC prices as they are computed in the background on every block where update occurred

## v0.16.0

- Pricing options; pricing source wiring at the app level
- Router options; remove GetOptimalQuoteFromConfig API.
- Fetch only required taker fees instead of all.
- Pre-allocate buffers in GetCandidateRoutes
- Unsafe cast in GetCandidateRoutes for performance
- Use slice instead of map in GetCandidateRoutes for performance
- More performance tricks in GetCandidateRoutes
- Cache zero routes for lower TTL if none found between token A and B
- Validate chain denom parameters in /quote and /routes and /prices

## v0.15.0

- Sentry CORS config
- v24 import paths
- Speedup for Order of Magnitude
- Remove redundant allocations
- LRU cache for tick to sqrt price

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

* [#160](https://github.com/osmosis-labs/sqs/pull/160) Custom sampling rate per endpoint.

## v0.13.0

* [#158](https://github.com/osmosis-labs/sqs/pull/158)  Integrate Sentry & add new configs.

## v0.12.0

* [#143](https://github.com/osmosis-labs/sqs/pull/143) light clean ups from the data ingest refactor. 
* [#148](https://github.com/osmosis-labs/sqs/pull/148) white whale switch of base and quote denoms in spot price.
* [#147](https://github.com/osmosis-labs/sqs/pull/147) GRPC ingester config, code gen and wiring.

## v0.11.1

Attempt transmuter fix

## v0.11.0

Fix excessive price impact bug. 

## v0.10.0

* [#108](https://github.com/osmosis-labs/sqs/pull/107) Add code id (omitempty) to /quote route response
* [#107](https://github.com/osmosis-labs/sqs/pull/107) Invert spot price in quotes. Break quote API

## v0.9.1

Hot fix for white whale base quote confusion

## v0.9.0

- Support all asset list v1 tokens
- Use spot price in pricing

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

* [#100](https://github.com/osmosis-labs/sqs/pull/100) Format in over out spot price in quotes.

## v0.7.0

* [#99](https://github.com/osmosis-labs/sqs/pull/99) Move candidate routes cache out of Redis. Remove route overwrite

Various performance improvement optimizations.

## v0.6.0

* [#85](https://github.com/osmosis-labs/sqs/pull/85) /tokens/prices endpoint

## v0.5.0

 Human readable denom params in router
* [#84](https://github.com/osmosis-labs/sqs/pull/84) enable swagger
* [#83](https://github.com/osmosis-labs/sqs/pull/83) /tokens/metadata/:denom endpoint

## v0.4.0

* [#81](https://github.com/osmosis-labs/osmosis/pull/81) Add support for single quotes as param in /quote.
* Rename import paths to v23
* Change Osmosis dependency to point to v23.x branch

## v0.3.0

## v0.2.2

## v0.2.1

* [#76](https://github.com/osmosis-labs/osmosis/pull/76) Keep `/pools` endpoint only, allowing the IDs parameter

## v0.2.0

* [#75](https://github.com/osmosis-labs/osmosis/pull/75) Break /pools/all and introduce /pools?IDs

## v0.1.3

* [#52](https://github.com/osmosis-labs/osmosis/pull/52) Fix key separator issue breaking tokenfactory denoms.
* [#53](https://github.com/osmosis-labs/osmosis/pull/53) Fix /version query
* [#54](https://github.com/osmosis-labs/osmosis/pull/54) Return all paramters from /config (as opposed to just router config)
* [#64](https://github.com/osmosis-labs/osmosis/pull/64) refactor: GetTakerFee by pool ID - avoid getting all taker fees

## v0.1.2

* [#46](https://github.com/osmosis-labs/osmosis/pull/46) Various fixes around cosmwasm pool implementation after testing Astroport PCL on testnet.

## v0.1.1

* [#45](https://github.com/osmosis-labs/osmosis/pull/45) Fix build issue by updating sqsdomain dep.
Update config value.

## v0.1.0

* [#43](https://github.com/osmosis-labs/osmosis/pull/43) Implement generalized CosmWasm pools into router that interact with the chain for computing quotes. Expose spot price by pool ID API.