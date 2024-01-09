# Contributing Guidelines

## Branch Management

While in [osmosis](https://github.com/osmosis-labs/osmosis), we develop against a `main` branch and then backport to release branches such as [v21.x](https://github.com/osmosis-labs/osmosis/tree/v21.x).

Each release branch and the main branch might rely on incompatible dependencies.
Therefore, we must maintain parity between SQS and Osmosis branches.

While in `osmosis`, `main` is the default working branch, this is not the case in `sqs`.

Here, we always develop on the currently live major branch (e.g. `v21.x`) by default but keep backporting with [labels](https://github.com/osmosis-labs/sqs/tree/main/.github/mergify.yml) to `main` branch. 

Once chain upgrades to the next major, cut a new release branch (e.g. `v22.x` ) from `main` in `sqs`,
make `v22.x` as default and completely drop the old `v21.x` while continuing to backport to `main`.

To sum up,
- `sqs/vx.x` always references `osmosis/v.x.x`
   * This is the default repository branch
- `sqs/main` always references `osmosis/main`
   * This is the branch we maintain to cut the new major branch once chain release happens

