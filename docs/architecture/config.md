# Configuration

We use a flexible configuration system that allows for easy customization through both configuration files and environment variables.
This document outlines how configuration values are set and overridden.

The service has a default configuration set in `domain/config.go:DefaultConfig`.

## Configuration Priority

The configuration values are determined in the following order of precedence (highest to lowest):

1. Environment Variables
2. Configuration File
3. Default Values

This means that environment variables will override values set in the configuration file, which in turn override the default values.

## Configuration File

The configuration file is typically a JSON file that contains all the configurable parameters for SQS.
The default location for this file is `/osmosis/config.json` within the Docker container.

## Environment Variable Overrides

Any configuration parameter can be overridden using environment variables. The system automatically maps configuration keys to environment variables using the following rules:

1. The environment variable names are prefixed with `SQS_`.
2. The configuration key is converted to uppercase.
3. Nested keys are separated by underscores.
4. Dashes in key names are replaced with underscores.

### Example Mappings

| Config Key | Environment Variable |
|------------|----------------------|
| `server-address` | `SQS_SERVER_ADDRESS` |
| `logger.filename` | `SQS_LOGGER_FILENAME` |
| `router.max-pools-per-route` | `SQS_ROUTER_MAX_POOLS_PER_ROUTE` |

## Usage

### Using a Configuration File

When running the SQS Docker container, mount your configuration file to `/osmosis/config.json`:

```bash
docker run -d --name sqs \
  -v /path/to/your/config.json:/osmosis/config.json:ro \
  -p 9092:9092 \
  osmolabs/sqs:local \
  -config /osmosis/config.json
```

### Using Environment Variables

```bash
docker run -d --name sqs \
  -e SQS_SERVER_ADDRESS=":9093" \
  -e SQS_LOGGER_LEVEL="debug" \
  -e SQS_ROUTER_MAX_POOLS_PER_ROUTE="5" \
  -p 9093:9093 \
  osmolabs/sqs:local
```

### Combining Configuration File and Environment Variables

You can use both a configuration file and environment variables.
The environment variables will override any values set in the file.

```bash
docker run -d --name sqs \
  -v /path/to/your/config.json:/osmosis/config.json:ro \
  -e SQS_SERVER_ADDRESS=":9093" \
  -p 9093:9093 \
  osmolabs/sqs:local \
  -config /osmosis/config.json
```
