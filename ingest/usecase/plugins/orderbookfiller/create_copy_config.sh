#!/bin/bash

# Define the input and output file paths
INPUT_FILE="config.json"   # Replace with the actual file path
OUTPUT_FILE="config_orderbook_fill_plugin.json"

# Use jq to modify the JSON and create a new file
jq '. 
    | .["grpc-tendermint-rpc-endpoint"] = "http://osmosis:26657" 
    | .["grpc-gateway-endpoint"] = "osmosis:9090" 
    | .otel.environment = "sqs-fill-bot" 
    | .["grpc-ingester"].plugins[] |= if .name == "orderbook" then .enabled = true else . end' \
    "$INPUT_FILE" > "$OUTPUT_FILE"

echo "Modified configuration saved to $OUTPUT_FILE"
