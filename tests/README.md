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