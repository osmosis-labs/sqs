# CosmWasm Pools

To calculate token out from token in with CosmWasm pool, SQS can query the pool contract directly from the node to get those information. But that will lead to slow response. That's why there are `general-cosmwasm-code-ids` in the config to determine that these code ids do not have specific logic to optimize price calculation and will be opted out from route calculation.

On the other hand, there are simpler pools like `transmuter` v1 which always have 1:1 ratio between token in and token out. So there is no need for more information to calculate token out given in, all that is needed are `transmuter-code-ids`.

But if we want to have more complex logic for price calculation and keep the response time fast, we can ingest enough information to SQS to calculate token out given in & spot price. This is where `CosmWasmPoolModel` comes in.

```go
// CosmWasm contract info from [cw2 spec](https://github.com/CosmWasm/cw-minus/blob/main/packages/cw2/README.md)
type ContractInfo struct {
	Contract string `json:"contract"`
	Version  string `json:"version"`
}

// CosmWasmPoolModel is a model for the pool data of a CosmWasm pool
// It includes the contract info and the pool data
// The CWPoolData works like a tagged union to hold different types of data
// depending on the contract and its version
type CosmWasmPoolModel struct {
	ContractInfo ContractInfo `json:"contract_info"`
	Data         CWPoolData   `json:"data"`
}
```

Within SQS, we want to have enough information to calculate token out given in & spot price. And by design, SQS can obtain those information from ingester every block.

Ingestor query the contract to determine which type of pool it is using [cw2 spec](https://github.com/CosmWasm/cw-minus/blob/main/packages/cw2/README.md). This is better option than hard coding code ids on the ingestor side since each environment can have different code ids for the same contract but cw2 info should be consistent throughtout.

And then query and transform necessary data to fit into `CWPoolData` struct which works like a tagged union to hold different types of data depending on the contract and its version. If there is a new pool type, we can add new type to `CWPoolData` and update the ingester to transform the data accordingly.

```go
// CosmWasmPoolData is the custom data for each type of CosmWasm pool
// This struct is intended to work like tagged union in other languages
// so that it can hold different types of data depending on the contract
type CosmWasmPoolData struct {
	// Data for AlloyTransmuter contract, must be present if and only if `IsAlloyTransmuter()` is true
	AlloyTransmuter *AlloyTransmuterData `json:"alloy_transmuter,omitempty"`

	// Data for Orderbook contract, must be present if and only if `IsOrderbook()` is true
	Orderbook *OrderbookData `json:"orderbook,omitempty"`

	// More pool types here...
}
```

With those information, we can construct `RoutablePool` to facilitate the calculation of token out given in & spot price specific to those pool type.

One caveat on utilizing cw2 information is that there is no uniqueness check for these contract info. But this should be manageable since not excessive amount of pool type is expected and cosmwasm pool are permissioned.


## Transmuter

`crates.io:transmuter` `<3.0.0`: requires no additional information.
`crates.io:transmuter` `>=3.0.0`: requires alloyed asset denom and normalization factors for each asset.

### Alloyed

[Alloyed Transmuter Pools](https://forum.osmosis.zone/t/alloyed-assets-on-osmosis-unifying-ux-and-solving-liquidity-fragmentation/2624) is a novel mechanism developed
by Osmosis to unify multiple bridged assets into a single LP share token.

Alloyed assets represent LP shares of these pools, combining various asset versions into one fungible token. Depositing different asset versions into the pool adds liquidity and, in return, users receive LP shares in the form of alloyed assets, enhancing UX by consolidating liquidity.

Users can acquire alloyed assets by simply swapping a like-kind asset for the alloyed version in the swap page.

Swapping into the alloy is equivalent to depositing the asset into the pool and receiving the LP share token. Swapping out of the alloy is equivalent to burning the LP share token and receiving the asset.

In the context of SQS router, this poses a number of challenges. The router needs to be aware of the alloyed asset denom.

For example, the alloyed asset denom is not contained in the pool balances or denoms of the pools that mints it as an LP share. Without custom handling, the router would be unable to consider the alloyed asset denom in route finding
This stems from the fact that the route finding algorithm looks at the pool denoms as well as how much liquidity a denom contributes to.

To address this, we define the following invariants for alloyed assets for the context of SQS:
- Each pool that mints alloyed asset denom does not contain it in balances but we include it in the pool denoms.
- The pool that mints alloyed asset denom does not include the alloyed asset value in its liquidity capitalization.
   * Example: [Pool 1868](https://app.osmosis.zone/pool/1868) and allBTC.
- The pool that does not mint alloyed asset but contains it one of the direct assets includes the alloyed asset value in its liquidity capitalization as well as in the balances
   * Example: [Pool 1835](https://app.osmosis.zone/pool/1835) and USDT.
- During ingest, we maintain the [BlockPoolMetaData.DenomPoolLiquidityMap](https://github.com/osmosis-labs/sqs/blob/d32f6a1ef6fd2a081f60f1f510023a0c8f9b1530/docs/architecture/ingest.md#L37-L49) to keep track of the liquidity capitalization for each denom across all pools.
  * We add the alloyed asset value to the liquidity capitalization contribution for non-minting pools that contain an LP share as one of the direct assets.
  * However, for `BlockPoolMetaData.DenomPoolLiquidityMap.Pools`, we add the LP share as contributing
  zero to liquidity capitalization. By maintaining the LP share denom in the `DenomPoolLiquidityMap.Pools`, we can ensure that the LP share is not double counted towards liquidity capitalization but still be able
  to find routes over the "minting" pools.

## Orderbook
`crates.io:sumtree-orderbook` `>= 0.1.0`: requires base and quote denoms, ticks liquidity, next bid and ask tick.

From there onwards, there might be requirement to update `RouteablePool` to have updated information and logic.
We do not put the constraint on the maximum version supported here since it will require code changes even though
there is no change in logic. And it already have whitelist for supported pool type by the code ids for the moment.
