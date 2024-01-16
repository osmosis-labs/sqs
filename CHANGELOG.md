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