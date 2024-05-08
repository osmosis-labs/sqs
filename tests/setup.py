import copy
from data_service import all_tokens_data, all_pools_data
from enum import Enum, IntEnum

# Numia pool type constants using an Enum
class NumiaPoolType(Enum):
    BALANCER = 'osmosis.gamm.v1beta1.Pool'
    STABLESWAP = 'osmosis.gamm.poolmodels.stableswap.v1beta1.Pool'
    CONCENTRATED = 'osmosis.concentratedliquidity.v1beta1.Pool'
    COSMWASM = 'osmosis.cosmwasmpool.v1beta1.CosmWasmPool'

# Cosmwasm pool code IDs using standard constants
TRANSMUTER_CODE_ID = 148
ASTROPORT_CODE_ID = 773

# Local e2e pool types using an IntEnum for convenience
class E2EPoolType(IntEnum):
    BALANCER = 0
    STABLESWAP = 1
    CONCENTRATED = 2
    COSMWASM_MISC = 3
    COSMWASM_TRANSMUTER_V1 = 4
    COSMWASM_ASTROPORT = 5

# Mapping from Numia pool types to e2e pool types
NUMIA_TO_E2E_MAP = {
    NumiaPoolType.BALANCER.value: E2EPoolType.BALANCER,
    NumiaPoolType.STABLESWAP.value: E2EPoolType.STABLESWAP,
    NumiaPoolType.CONCENTRATED.value: E2EPoolType.CONCENTRATED
}

# Misc constants
UOSMO = "uosmo"

def get_e2e_pool_type_from_numia_pool(pool):
    """Gets an e2e pool type from a Numia pool."""
    numia_pool_type = pool.get("type")

    # Direct mapping for common pool types
    if numia_pool_type in NUMIA_TO_E2E_MAP:
        return NUMIA_TO_E2E_MAP[numia_pool_type]

    # Special handling for CosmWasm pools based on code_id
    if numia_pool_type == NumiaPoolType.COSMWASM.value:
        pool_code_id = int(pool.get('code_id'))
        if pool_code_id == TRANSMUTER_CODE_ID:
            return E2EPoolType.COSMWASM_TRANSMUTER_V1
        elif pool_code_id == ASTROPORT_CODE_ID:
            return E2EPoolType.COSMWASM_ASTROPORT
        else:
            return E2EPoolType.COSMWASM_MISC

    # Raise an error for unknown pool types
    raise ValueError(f"Unknown pool type: {numia_pool_type}")

def get_denoms_from_pool_tokens(pool_tokens):
    """
    Extracts and returns the list of denoms from the `pool_tokens` field, 
    handling both dictionary (asset0/asset1) and list formats.
    """
    denoms = []
    
    # Concentrated pool type
    if isinstance(pool_tokens, dict):  # Dictionary case
        denoms.extend([pool_tokens.get('asset0', {}).get('denom'), pool_tokens.get('asset1', {}).get('denom')])
        denoms = [denom for denom in denoms if denom]  # Remove None values

    # All other types
    elif isinstance(pool_tokens, list):  # List case
        denoms = [token.get('denom') for token in pool_tokens if 'denom' in token]

    return denoms


def map_pool_type_to_pool_data(pool_data):
    """
    Returns a dictionary mapping each pool type to associated ID, liquidity, and tokens.

    Example output:
    {
        "e2e_pool_type_1": [[pool_id, liquidity, [denoms]]],
        "e2e_pool_type_2": [[pool_id, liquidity, [denoms]], ...],
        ...
    }
    """
    if not pool_data:
        return {}

    pool_type_to_data = {}

    for pool in pool_data:
        # Convert Numia pool type to e2e pool type
        e2e_pool_type = get_e2e_pool_type_from_numia_pool(pool)

        # Extract denoms using a helper function
        pool_tokens = pool.get("pool_tokens")
        denoms = get_denoms_from_pool_tokens(pool_tokens)

        # Extract pool ID and liquidity
        pool_id = pool.get('pool_id')
        liquidity = pool.get('liquidity')

        # Initialize the pool type if not already done
        if e2e_pool_type not in pool_type_to_data:
            pool_type_to_data[e2e_pool_type] = []

        # Append the pool data to the list for this pool type
        pool_type_to_data[e2e_pool_type].append([pool_id, liquidity, denoms])

    return pool_type_to_data

def create_display_to_data_map(tokens_data):
    """Function to map display field to the data of that token."""

    display_map = {}
    for token in tokens_data:
        display_field = token.get('display')
        if display_field:
            display_map[display_field] = token
    return display_map

def create_chain_denom_to_data_map(tokens_data):
    """Function to map chain denom to the data of that token."""

    display_map = {}
    for token in tokens_data:
        display_field = token.get('denom')
        if display_field:
            display_map[display_field] = token
    return display_map

def get_token_data_copy():
    """Return deep copy of all tokens."""
    return copy.deepcopy(all_tokens_data)


def choose_tokens_liq_range(num_tokens=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose tokens based on liquidity."""
    tokens = get_token_data_copy()
    filtered_tokens = [
        t['denom'] for t in tokens if 'liquidity' in t and t['liquidity'] is not None and min_liq <= t['liquidity'] <= max_liq
    ]
    sorted_tokens = sorted(
        filtered_tokens, key=lambda x: next(t['liquidity'] for t in tokens if t['denom'] == x), reverse=not asc
    )
    return sorted_tokens[:num_tokens]

def choose_tokens_volume_range(num_tokens=1, min_vol=0, max_vol=float('inf'), asc=False):
    """Function to choose tokens based on volume."""

    tokens = get_token_data_copy()
    filtered_tokens = [
        t['denom'] for t in tokens if 'volume_24h' in t and t['volume_24h'] is not None and min_vol <= t['volume_24h'] <= max_vol
    ]
    sorted_tokens = sorted(
        filtered_tokens, key=lambda x: next(t['volume_24h'] for t in tokens if t['denom'] == x), reverse=not asc
    )
    return sorted_tokens[:num_tokens]

def choose_pool_type_tokens_by_liq_asc(pool_type, num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a specific pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    
    pools_tokens_of_type = pool_type_to_denoms.get(pool_type)

    sorted_pools = sorted(pools_tokens_of_type, key=lambda x: x[1], reverse=not asc)

    return [[pool_data[1], pool_data[2]] for pool_data in sorted_pools[:num_pairs]]

def choose_transmuter_pool_tokens_by_liq_asc(num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a transmuter V1 pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(E2EPoolType.COSMWASM_TRANSMUTER_V1, num_pairs, min_liq, max_liq, asc)

def choose_pcl_pool_tokens_by_liq_asc(num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a Astroport PCL pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(E2EPoolType.COSMWASM_ASTROPORT, num_pairs, min_liq, max_liq, asc)

def chain_denom_to_display(chain_denom):
    """Function to map chain denom to display."""
    return chain_denom_to_data_map.get(chain_denom, {}).get('display', chain_denom)

def chain_denoms_to_display(chain_denoms):
    """Function to map chain denoms to display."""
    return [chain_denom_to_display(denom) for denom in chain_denoms]

# Create a map of display to token data
display_to_data_map = create_display_to_data_map(all_tokens_data)

# Create a map of chain denom to token data
chain_denom_to_data_map = create_chain_denom_to_data_map(all_tokens_data)

# Create a map of pool type to pool data
pool_type_to_denoms = map_pool_type_to_pool_data(all_pools_data)
