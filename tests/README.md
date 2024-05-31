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

## CI Integration

Our integration suite is run as a GitHub Action [integration-test.yml](https://github.com/osmosis-labs/sqs/blob/d53c34806bafe3162d493f3d51bffd439371a7a0/.github/workflows/integration-test.yml).

There are 3 possible triggers:

1. Manual trigger
* Option to select the environment to run the tests against.

2. Hourly Schedule
* Runs across all supported environments.

3. Post auto-deployment to stage ([TBD](https://linear.app/osmosis/issue/PLAT-207/sqs-stage-deployment-completion-hook-in-ci))

### Supported Environmnets

- `prod` -> `https://sqs.osmosis.zone`
- `stage` -> `https://sqs.stage.osmosis.zone`

### Environment Variables

Our `pytest` parses the following environment variables in `conftest.py`

- `SQS_API_KEY` -> API Key to bypass rate limite. If not provided, the tests will run without api key set.
- `SQS_ENVIRONMENTS` -> Comma separated list of environment names per "Supported Environments" to run the tests against. If not provided, the tests will run against stage.
