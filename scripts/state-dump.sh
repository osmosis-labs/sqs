#!/usr/bin/env bash

set -e

# Function to make a POST request and move the resulting file
post_and_move() {
	local HOSTNAME=$1
	local ENDPOINT=$2
	local FILE=$3
	local DEST=$4

	if [[ -z "$HOSTNAME" || -z "$ENDPOINT" || -z "$FILE" || -z "$DEST" ]]; then
		echo "Usage: post_and_move <hostname> <endpoint> <file> <destination_file>" >&2
		return 1
	fi

	if ! curl -sS -X POST "${HOSTNAME}${ENDPOINT}" -o /dev/null; then
		echo "Error: curl request to ${HOSTNAME}${ENDPOINT} failed." >&2
		return 1
	fi

	if [[ ! -f "$FILE" ]]; then
		echo "Error: File '$FILE' not found after curl." >&2
		return 1
	fi

	mv "$FILE" "$DEST"
	echo "Moved $FILE to $DEST"
}

HOSTNAME="http://localhost:9092"
ROUTER_ENDPOINT="/router/store-state"
TOKENS_ENDPOINT="/tokens/store-state"
OUTPUT_DIR="${1:-./state}"

echo "Starting state dump..."

mkdir -p "$OUTPUT_DIR"

post_and_move "$HOSTNAME" "$ROUTER_ENDPOINT" "pools.json" "${OUTPUT_DIR}/pools.json"
post_and_move "$HOSTNAME" "$ROUTER_ENDPOINT" "taker_fees.json" "${OUTPUT_DIR}/taker_fees.json"
post_and_move "$HOSTNAME" "$ROUTER_ENDPOINT" "candidate_route_search_data.json" "${OUTPUT_DIR}/candidate_route_search_data.json"

post_and_move "$HOSTNAME" "$TOKENS_ENDPOINT" "tokens.json" "${OUTPUT_DIR}/tokens.json"
post_and_move "$HOSTNAME" "$TOKENS_ENDPOINT" "pool_denom_metadata.json" "${OUTPUT_DIR}/pool_denom_metadata.json"

echo "State dump completed successfully."
