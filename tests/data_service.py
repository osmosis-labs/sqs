import requests
from decimal import Decimal

import conftest

# Endpoint URLs
NUMIA_API_URL = 'https://data.stage.osmosis.zone'
TOKENS_ENDPOINT = '/tokens/v2/all'
POOLS_ENDPOINT = '/stream/pool/v1/all?min_liquidity=0'

def fetch_tokens():
    """Fetches all tokens from the specified endpoint and returns them."""
    url = NUMIA_API_URL + TOKENS_ENDPOINT
    try:
        response = requests.get(url)
        response.raise_for_status()  # Raise an error for unsuccessful requests
        tokens = response.json()
        return tokens
    except requests.exceptions.RequestException as e:
        print(f"Error fetching data from Numia: {e}")
        return []
    
# Helper function to calcualte token cap given the token amount 
# and the token price from coingecko
def get_token_cap(denom, amount_str):
    token_metadata = conftest.SERVICE_ASSET_LIST.get_asset_metadata(denom)
    if token_metadata is None:
        return None
    if token_metadata.get('coingeckoId') is None:
        return None
    token_price = conftest.SERVICE_COINGECKO.get_token_price(token_metadata.get('coingeckoId'))
    if token_price is None:
        return None
    return Decimal(amount_str) * Decimal(token_price) 

# Helper function to update the pool liquidity cap using Coingecko
# abort the update if any token cap is not available
def update_pool_liquidity_cap(pool):
    try:
        total_pool_cap = Decimal(0)
        all_token_cap_captured = True # flag to check if all token cap is captured, if not, no update will be made
        tokens = pool.get('pool_tokens', []) 
        if not isinstance(tokens, list): # tokens from numia can be a dict or a list
            tokens = tokens.values()
        for token in tokens:
            denom = token.get('denom')
            token_cap = get_token_cap(denom, token['amount'])
            if token_cap is None:
                all_token_cap_captured = False
                break
            total_pool_cap += token_cap
        if all_token_cap_captured:
            pool.update({'liquidity': float(total_pool_cap)})
    except Exception as e:
        print(f"warning: error processing pool data (pool id {pool.get('pool_id')}) using Coingecko: {e}. fallback to numia data")
        return

def fetch_pools():
    """Fetches all pools by iterating through paginated results."""
    url = NUMIA_API_URL + POOLS_ENDPOINT
    all_pools = []
    next_offset = 0
    batch_size = 100  # Adjust if a different pagination size is needed

    while True:
        params = {'offset': next_offset, 'limit': batch_size}
        try:
            response = requests.get(url, params=params)
            response.raise_for_status()
            data = response.json()
            pools = data.get('pools', [])
            pagination = data.get('pagination', {})

            # Calculate the pool liquidity cap using Coingecko if service is available, aka the API key is provided thru env var
            if conftest.SERVICE_COINGECKO.isServiceAvailable():
                # Iterate through the pools and update the **existing** pool liquidity cap data with a more accurate value from Coingecko
                for pool in pools:
                    for pool in pools:
                        update_pool_liquidity_cap(pool)

            # Add this batch to the accumulated pool data
            all_pools.extend(pools)

            # Determine if more pools are available
            next_offset = pagination.get('next_offset')
            if not next_offset:
                break

        except requests.exceptions.RequestException as e:
            print(f"Error fetching data from Numia: {e}")
            break

    return all_pools
