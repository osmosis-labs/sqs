import requests

# Endpoint URLs
NUMIA_API_URL = 'https://data.numia-stage.osmosis.zone'
TOKENS_ENDPOINT = '/tokens/v2/all'
POOLS_ENDPOINT = '/stream/pool/v1/all'

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
