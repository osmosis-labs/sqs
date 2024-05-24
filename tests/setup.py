import copy
import itertools
from data_service import all_tokens_data, all_pools_data
from conftest import SERVICE_SQS_STAGE
from enum import Enum, IntEnum
from constants import *

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

# This is the number of tokens we allow being skipped due to being unlisted
# or not having liquidity. This number is hand-picked arbitrarily. We have around ~350 tokens
# at the time of writing this test and we leave a small buffer.
ALLOWED_NUM_TOKENS_USDC_PAIR_SKIPPED = 50
# This is the minimum number of misc token pairs
# that we expect to construct in setup.
# Since our test setup is dynamic, it is important to
# validate we do not get false positve passes due to small
# number of pairs constructed.
MIN_NUM_MISC_TOKEN_PAIRS = 10

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


def create_pool_data_maps(pool_data):
    """
    Creates and returns pool data dictionaries for testing.

    Returns two dictionaries:
     
    1. A dictionary mapping each pool type to associated ID, liquidity, and tokens.

    Example output:
    {
        "e2e_pool_type_1": [[pool_id, liquidity, [denoms]]],
        "e2e_pool_type_2": [[pool_id, liquidity, [denoms]], ...],
        ...
    }

    2. A dictionary mapping each denom to the pool with highest liquidity

    Example output:
    {
        "uosmo": { "pool_liquidity": 1000000, "pool_id": 1 },
        ...
    }
    """
    if not pool_data:
        return {}

    pool_type_to_data = {}

    denom_top_liquidity_pool_map = {}

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

        for denom in denoms:
            denom_pool_data = denom_top_liquidity_pool_map.get(denom)

            if denom_pool_data:
                # Update the pool ID if the liquidity is higher
                if liquidity > denom_pool_data['pool_liquidity']:
                    denom_pool_data['pool_liquidity'] = liquidity
                    denom_pool_data['pool_id'] = pool_id
            else:
                # Create first mapping for this denom
                denom_top_liquidity_pool_map[denom] = {'pool_liquidity': liquidity, 'pool_id': pool_id}

    return pool_type_to_data, denom_top_liquidity_pool_map


def create_field_to_data_map(tokens_data, key_field):
    """Maps a specified key field to the data of that token.

    Args:
        tokens_data (list): List of token data dictionaries.
        key_field (str): The field to use as the mapping key.

    Returns:
        dict: A dictionary mapping the specified key field to the token data.
    """
    mapping = {}
    for token in tokens_data:
        key_value = token.get(key_field)
        if key_value:
            mapping[key_value] = token
    return mapping


def create_display_to_data_map(tokens_data):
    return create_field_to_data_map(tokens_data, 'display')


def create_chain_denom_to_data_map(tokens_data):
    return create_field_to_data_map(tokens_data, 'denom')


def get_token_data_copy():
    """Return deep copy of all tokens."""
    return copy.deepcopy(all_tokens_data)


def choose_tokens_generic(tokens, filter_key, min_value, max_value, sort_key, num_tokens=1, asc=False):
    """
    A generic function to choose tokens based on given criteria.

    Args:
        tokens (list): The list of token data dictionaries.
        filter_key (str): The field name used to filter tokens.
        min_value (float): The minimum value for filtering.
        max_value (float): The maximum value for filtering.
        sort_key (str): The field name used for sorting tokens.
        num_tokens (int): The number of tokens to return.
        asc (bool): Whether to sort in ascending order.

    Returns:
        list: A list of denoms matching the given criteria.
    """
    # Filter tokens based on the specified filter_key range
    filtered_tokens = [
        t['denom'] for t in tokens if filter_key in t and t[filter_key] is not None and min_value <= t[filter_key] <= max_value
    ]

    # Sort tokens based on the specified sort_key
    sorted_tokens = sorted(
        filtered_tokens, key=lambda x: next(t[sort_key] for t in tokens if t['denom'] == x), reverse=not asc
    )

    return sorted_tokens[:num_tokens]


def choose_tokens_liq_range(num_tokens=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose tokens based on liquidity."""
    tokens = get_token_data_copy()
    return choose_tokens_generic(tokens, 'liquidity', min_liq, max_liq, 'liquidity', num_tokens, asc)


def choose_tokens_volume_range(num_tokens=1, min_vol=0, max_vol=float('inf'), asc=False):
    """Function to choose tokens based on volume."""
    tokens = get_token_data_copy()
    return choose_tokens_generic(tokens, 'volume_24h', min_vol, max_vol, 'volume_24h', num_tokens, asc)


def choose_pool_type_tokens_by_liq_asc(pool_type, num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """
    Function to choose pool ID and tokens associated with a specific pool type based on liquidity.

    Args:
        pool_type (E2EPoolType): The pool type to filter by.
        num_pairs (int): The number of pool pairs to return.
        min_liq (float): The minimum liquidity value.
        max_liq (float): The maximum liquidity value.
        asc (bool): Whether to sort in ascending or descending order.

    Returns:
        list: [[pool ID, [tokens]], ...]
    """
    # Retrieve pools associated with the specified pool type
    pools_tokens_of_type = pool_type_to_denoms.get(pool_type, [])

    # Filter pools based on the provided min_liq and max_liq values
    filtered_pools = [
        pool for pool in pools_tokens_of_type if min_liq <= pool[1] <= max_liq
    ]

    # Sort the filtered pools based on liquidity
    sorted_pools = sorted(filtered_pools, key=lambda x: x[1], reverse=not asc)

    # Extract only the required number of pairs
    return [[pool_data[0], pool_data[2]] for pool_data in sorted_pools[:num_pairs]]


def choose_transmuter_pool_tokens_by_liq_asc(num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a transmuter V1 pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(E2EPoolType.COSMWASM_TRANSMUTER_V1, num_pairs, min_liq, max_liq, asc)


def choose_pcl_pool_tokens_by_liq_asc(num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a Astroport PCL pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(E2EPoolType.COSMWASM_ASTROPORT, num_pairs, min_liq, max_liq, asc)

def choose_valid_listed_tokens():
    """
    Returns all listed tokens from the asset list that have at least one pool with liquidity.

    Queries production SQS for the tokens asset list metadata.

    Queries Numia for the pool liquidity data.
    """

    # We rely on SQS itself for getting the tokens metadata for configuring tests.
    # While it is not the best practice, we make an exception since this is the most reliable way to get
    # The asset list data. In the future, we can implement custom test parsing to replace relying on SQS
    # in test setup.
    tokens_metadata = SERVICE_SQS_STAGE.get_tokens_metadata()

    if len(tokens_metadata) == 0:
        raise ValueError("Error: no tokens metadata retrieved from SQS during tokens setup")
    
    valid_listed_tokens = []

    for denom, metadata in tokens_metadata.items():
        # Skip unlisted tokens as they should be unsupported
        # in SQS.
        if metadata['preview']:
            [print(f"Denom {denom} is unlisted")]
            continue

        # Skip tokens with no pools with liquidity as we cannot find routes in-between them.
        # Note: if tests prove to be flaky due to pools with low liq > 10 but < min liq filter, we can
        # dynamically set the min liquidity filter by querying the config.
        top_liquidity_pool = denom_top_liquidity_pool_map.get(denom)
        if top_liquidity_pool is None or top_liquidity_pool['pool_liquidity'] == 0:
            print(f"Denom {denom} has no pool with liquidity")
            continue

        valid_listed_tokens.append(denom)

    skipped_token_count = len(tokens_metadata) - len(valid_listed_tokens)
    if skipped_token_count > ALLOWED_NUM_TOKENS_USDC_PAIR_SKIPPED:
        raise ValueError(f"Too many tokens {skipped_token_count} from the metadata were untested, allowed {ALLOWED_NUM_TOKENS_USDC_PAIR_SKIPPED} tokens to be skipped")

    return valid_listed_tokens


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

# Create two maps:
# 1. A map of pool type to pool data
# 2. A map of denom to top liquidity pool
pool_type_to_denoms, denom_top_liquidity_pool_map = create_pool_data_maps(all_pools_data)

# Create a map of pool ID to pool data
pool_by_id_map = {pool.get('pool_id'): pool for pool in all_pools_data}

# Listed tokens that have at least one pool with liquidity
valid_listed_tokens = choose_valid_listed_tokens() 

# One Transmuter token pair [[pool_id, ['denom0', 'denom1']]]
transmuter_token_pairs = choose_transmuter_pool_tokens_by_liq_asc(1)

# One Astroport token pair [[pool_id, ['denom0', 'denom1']]]
astroport_token_pair = choose_pcl_pool_tokens_by_liq_asc(1)

def create_token_pairs():
    """
    Selects the following groups of tokens:
    1. Top NUM_TOKENS_DEFAULT by-liquidity
    2. Top NUM_TOKENS_DEFAULT by-volume
    3. Five low liquidity (between MIN_LIQ_FILTER_DEFAULT and MAX_VAL_LOW_LIQ_FILTER_DEFAULT USD)
    4. Five low volume (between MIN_VOL_FILTER_DEFAULT and MAX_VAL_LOW_LIQ_FILTER_DEFAULT USD)

    Then,
    - Puts them all in a set
    - Constructs combinations between each.

    Returns combinations in the following format:
    [['denom0', 'denom1']]
    """

    # Five top by-liquidity tokens
    top_five_liquidity_tokens = choose_tokens_liq_range(NUM_TOKENS_DEFAULT)

    # NUM_TOKENS_DEFAULT top by-volume tokens
    top_five_volume_tokens = choose_tokens_volume_range(NUM_TOKENS_DEFAULT)

    # NUM_TOKENS_DEFAULT low liquidity tokens
    five_low_liquidity_tokens = choose_tokens_liq_range(NUM_TOKENS_DEFAULT, MIN_LIQ_FILTER_DEFAULT, MAX_VAL_LOW_LIQ_FILTER_DEFAULT)

    # NUM_TOKENS_DEFAULT low volume tokens
    five_low_volume_tokens = choose_tokens_volume_range(NUM_TOKENS_DEFAULT, MIN_VOL_FILTER_DEFAULT, MAX_VAL_LOW_VOL_FILTER_DEFAULT)

    # Put all tokens in a set to ensure uniqueness
    all_tokens = set(top_five_liquidity_tokens + top_five_volume_tokens +
                     five_low_liquidity_tokens + five_low_volume_tokens)

    # Construct all unique combinations of token pairs
    token_pairs = list(itertools.combinations(all_tokens, 2))

    # Format pairs for return
    formatted_pairs = [[token1, token2] for token1, token2 in token_pairs]

    if len(formatted_pairs) > MIN_NUM_MISC_TOKEN_PAIRS:
        ValueError(f"Constructeed {len(formatted_pairs)}, min expected {MIN_NUM_MISC_TOKEN_PAIRS}")

    return formatted_pairs

misc_token_pairs = create_token_pairs()
