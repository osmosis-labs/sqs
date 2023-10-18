# Sidecar Query Server (SQS)

## To Run

- Ensure that Redis is running locally
  * Instructions TBD
- Ensure that localosmosis is running locally

```
make run
```
- Uses the `config.json` para

## TODOs

- Set-up docker-compose that starts up all test services: sqs, localosmosis and redis
- Reduce code duplication in pool repository tests by using Generics
- Consider separate balancer and stableswap pools to be written to separate indexes
- Switch to using custom pool models and avoid writing chain pool models to state
- Tests
  - Refactor & test worker that pulls data from chain
  - Test pools use case
  - Test pools delivery


## Questions

- Requests for @jonator

- List all required fields for concentrated pool
- List all required fields for balancer pool
- List all required fields for cosmwasm pool
