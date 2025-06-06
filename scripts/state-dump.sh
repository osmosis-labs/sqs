#!/usr/bin/env bash

set -euo pipefail

HOSTNAME="http://localhost:9092"
ROUTER_ENDPOINT="/router/store-state"
TOKENS_ENDPOINT="/tokens/store-state"
OUTPUT_DIR="${1:-./state}"

EXPECTED_ROUTER_FILES=(
    "pools.json"
    "taker_fees.json"
    "candidate_route_search_data.json"
)

EXPECTED_TOKENS_FILES=(
    "tokens.json"
    "pool_denom_metadata.json"
)

echo "Starting state dump..."
mkdir -p "$OUTPUT_DIR"

# Function to move expected files
move_files() {
    local files=("$@")
    for file in "${files[@]}"; do
        if [[ ! -f "$file" ]]; then
            echo "Error: Expected file '$file' not found." >&2
            exit 1
        fi
        mv "$file" "${OUTPUT_DIR}/$file"
        echo "Moved $file to ${OUTPUT_DIR}/$file"
    done
}

# Request router state once
echo "Requesting router state..."
if ! curl -sS -X POST "${HOSTNAME}${ROUTER_ENDPOINT}" -o /dev/null; then
    echo "Error: curl request to ${HOSTNAME}${ROUTER_ENDPOINT} failed." >&2
    exit 1
fi
move_files "${EXPECTED_ROUTER_FILES[@]}"

# Request tokens state once
echo "Requesting tokens state..."
if ! curl -sS -X POST "${HOSTNAME}${TOKENS_ENDPOINT}" -o /dev/null; then
    echo "Error: curl request to ${HOSTNAME}${TOKENS_ENDPOINT} failed." >&2
    exit 1
fi
move_files "${EXPECTED_TOKENS_FILES[@]}"

echo "State dump completed successfully."
