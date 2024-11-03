# Orderbook Claimbot Plugin

The Orderbook Claimbot plugin is a plugin that claims filled order book orders.

It scans all active orders for each order book determining which orders have been filled and need to be claimed. At the moment order is said to be claimable if it is filled 98 percent or more. In order for an order book to be processed to claim its active orders it must be canonical as per SQS definition.


Such order book scanning and claiming is achieved by listening for new blocks and core logic is triggered at the end of each new block by calling Claimbot `ProcessEndBlock` method.

## Configuration

### Node

1. Initialize a fresh node with the `osmosisd` binary.
```bash
osmosisd init claim-bot --chain-id osmosis-1
```

2. Get latest snapshot from [here](https://snapshots.osmosis.zone/index.html)

3. Go to `$HOME/.osmosisd/config/app.toml` and set `osmosis-sqs.is-enabled` to true

4. Optionally, turn off any services from `app.toml` and `config.toml` that you don't need

### SQS

In `config.json`, set the plugin to enabled:

```json
"grpc-ingester":{
    ...
    "plugins": [
        {
            "name": "orderbook-claimbot-plugin",
            "enabled": true
        }
    ]
},
```

Configure the key on a test keyring, and set the following environment variables:
```bash
OSMOSIS_KEYRING_PATH=/root/.osmosisd/keyring-test
OSMOSIS_KEYRING_PASSWORD=test
OSMOSIS_KEYRING_KEY_NAME=local.info
```
- Here, the key is named `local` and the keyring path is in the default `osmosisd` home directory.

To create your key:
```bash
osmosisd keys add local --keyring-backend test --recover

# Enter your mnemonic

# Confirm the key is created
osmosisd keys list --keyring-backend test
```

Note that the test keyring is not a secure approach but we opted-in for simplicity and speed
of PoC implementation. In the future, this can be improved to support multiple backends.

## Starting (via docker compose)

1. Ensure that the "Configuration" section is complete.
2. From project root, `cd` into `ingest/usecase/plugins/orderbook/claimbot`
3. Update `.env` with your environment variables.
4. Run `make orderbook-claimbot-start`
5. Run `osmosisd status` to check that the node is running and caught up to tip.
6. Curl `/healthcheck` to check that SQS is running `curl http://localhost:9092/healthcheck`
