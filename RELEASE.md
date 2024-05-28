# Release Process

## Steps

This is a summary of the steps to get a change into production.

1. Merge sqs PR into main (v25.x) once approved
2. Create an RC tag e.g. `v0.20.0-rc0` and push it to the repo.
3. Create [infrastructure](https://github.com/osmosis-labs/infrastructure) PR for updating stage configs config.json.j2, sqs.yaml, if needed, as well as versions.yaml
4. Get infrastrucuture PR approved and merged into main
5. Deploy to stage (automated and triggered by step 4)
6. Test & request QA
7. Tag the non-RC release e.g. `v0.20.0` and push it to the repo
8. Repeat step 3 and 4 for prod
  * Reference the configuration from step 4.
9.  Manually perform prod deployment via Rundeck
  * Post updates in #eng-team-data-services.
     * Deployment start
     * Issues/blockers, if any
     * Deployment end

### Tagging RC

The rc numbers should be incremented for any changes that occur in staging and before getting to production.

Example:

Let's say that we plan a release, and the current version is v0.18.5. Then, we tag a v0.18.-6-rc0 and push it to the stage for testing. QA uncovers a bug during stage testing. We push a fix, incrementing the rc number for a new stage tag - v0.18.6-rc1. Now, the stage QA passes, and we proceed to production with v0.18.6.

## Versioning

We follow [semantic versioning](https://semver.org/).

For compatibility with the [chain releases](https://github.com/osmosis-labs/osmosis), we will increment the major version for any chain upgrade.

For any API, config breaking changes or new features, we aim to increment a minor version to reflect the breaking change.

For any minor bug fixes, non-breaking changes and small improvements, we increment the patch version.

Note that we will are tagging rc rather than the final tags for work that is still not ready for production and is being tested.

## Configuration

### General

The suggested configuration can be found:
- `config.json` for mainnet configuration.
- `config-testnet.json` for testnet configuration

### Internal

For Osmosis internal use, please always refer to the relevant
environment in the [infrastructure repository](https://github.com/osmosis-labs/infrastructure/tree/main/environments/sqs-osmosis-zone/environments/prod).
