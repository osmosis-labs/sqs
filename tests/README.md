# E2E Test

## Setup

```bash
make e2e-setup-venv

make e2e-source-venv

make e2e-install-requirements
```

## Running

```bash
# Runs the testsuite in verbose mode
make e2e-run-dev
```

### Isolate Specific Test

```bash
# Run a specific test from master node (one process)
 pytest -s tests/test_router_quote.py::TestQuote::test_usdc_in_high_liq_out[https://sqs.stage.osmosis.zone-62152071397ibc/69110FF673D70B39904FF056CFDFD58A90BEC3194303F45C32CB91B8B0A738EA]
```

## Persisting New Dependencies

In Python, we need to persist any new dependencies to the `requirements.txt` file. To do this, run the following command:

```bash
make e2e-update-requirements
```

## Using Debugger

It is possible to launch the test suite directly from VS Code.
We have a debug configuration under `.vscode/launch.json` called .

This configuration runs the test suite in verbose mode without parallelization.
It alows setting breakpoints for streamlined debugging.

## Test Suite Setup

There are 2 modes of running the suite:
1. Single-process
2. Multi-process

With the multi-process mode, the setup logic in `conftest.py` is executed by every worker.
Every worker then aggregates their results to construct parametrized tests and split them across each other.

As a result, the test parameters must be computed in a deterministic way. Our setup logic depends on
the external data provider (Numia). With the millisecond differences, in is possible to observe non-determinism.
To prevent that, we run the setup logic to generate common test parameters from a master process. We write
the output to a file. See `conftest.py::pytest_sessionstart` for more details.

We serialize the setup state as `conftest.py::SharedTestState`, letting each worker to then deserialize it
for deterministic test parameter generation.
