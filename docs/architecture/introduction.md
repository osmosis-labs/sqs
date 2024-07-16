# Introduction to Sidecar-Query-Server (SQS)

## Repository Layout

We use a [Clean Architecture](https://hackernoon.com/golang-clean-archithecture-efd6d7c43047) desing pattern
to group the code by separate "Functional Components" .

## Functional Components

### Pools

This is a component that deals with liqudiity pools and their operations.

See [docs/architecture/pools.md](https://github.com/osmosis-labs/sqs/blob/v25.x/docs/architecture/pools.md) for more details.

### Router Usecase

This is the routing & quoting component.

The core router implements `mvc.RouterUsecase` interface.

### Tokens UseCase (Prices and Metadata)

This is a component that deals with token metadata and prices.

Note that to be able to compute prices, we need access to the router.
However, we create a separate instances of the router that implements an `mvc.SimpleRouterUsecase`.

This allows us to utilize a separate configuration for the routers, decople the pricing
implementation that is meant to be more lightweight while minimizing code duplication. At the same
time, we improve debugging experience by introducing the ability to set breakpoints in separate entrypoints.
Additionally, we allow for caches to be split and one cache configuration & content not breaking the other.

### Chain Info

This is a component that deals with system resources and operations.

It contains:
- Healthcheck
- Profiling server endpoints