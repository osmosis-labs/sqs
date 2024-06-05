# CosmWasm Pools

To calculate token out from token in with CosmWasm pool, SQS can query the pool contract directly from the node to get those information. But that will lead to slow response. That's why there are `general-cosmwasm-code-ids` in the config to determine that these code ids does not have specific logic to optimize price calculation and will be opted out from route calculation.

On the other hand, there are simpler pools like `transmuter` v1 which always have 1:1 ratio between token in and token out. So there is no need more information to calculate token out given in, all it is needed are `transmuter-code-ids`.

But if we want to have more complex logic for price calculation and keep the response time fast, we can ingest enough information to SQS to calculate token out given in & spot price. This is where `CosmWasmPoolModel` come in.

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

Within SQS, we want to have enough information to calculate token out given in & spot price. And by design, SQS can obtain those information form ingester every block.

Ingestor query the contract to determine which type of pool it is using [cw2 spec](https://github.com/CosmWasm/cw-minus/blob/main/packages/cw2/README.md). This is better option than hard coding code ids on the ingestor side since each environment can have different code ids for the same contract but cw2 info should be consistent throughtout.

And then query and transform nessesary data to fit into `CWPoolData` struct which works like a tagged union to hold different types of data depending on the contract and its version. If there is a new pool type, we can add new type to `CWPoolData` and update the ingester to transform the data accordingly.

```go
// CWPoolData is the custom data for each type of CosmWasm pool
// This struct is intended to work like tagged union in other languages
// so that it can hold different types of data depending on the contract
type CWPoolData struct {
	// Data for AlloyTransmuter contract, must be present if and only if `IsAlloyTransmuter()` is true
	AlloyTransmuter *AlloyTransmuterData `json:"alloy_transmuter,omitempty"`

	// More pool types here...
}
```

With those information, we can construct `RoutablePool` to facilitate the calculation of token out given in & spot price specific to those pool type.

One caveat on utilizing cw2 information is that there is no uniqueness check for these contract info. But this should be managable since not excessive amount of pool type is expected and cosmwasm pool are permissioned.


## Transmuter

`crates.io:transmuter` `<3.0.0`: requires no additional information.
`crates.io:transmuter` `>=3.0.0`: requires alloyed asset denom and normalization factors for each asset.

From there onwards, there might be requirement to update `RouteablePool` to have updated information and logic.
We do not put the constraint on the maximum version supported here since it will require code changes even though
there is no change in logic. And it's already have whitelist for supported pool type by the code ids for the moment.
