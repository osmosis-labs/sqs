import copy
from filelock import FileLock
import pytest
import itertools
import json
import os

from sqs_service import *
from data_service import fetch_tokens, fetch_pools
from enum import Enum, IntEnum
from constants import *
from rand_util import construct_token_in_combos
from coingecko_service import *
from constants import *
from chain_service import ChainService
from util import *
from decimal import *
from asset_list_service import AssetListService




def parse_api_key():
    """
    Parse the API_KEY environment variable and return it

    If the environment variable is not set, the default API key is ""
    """
    api_key = os.getenv('SQS_API_KEY', None)
    
    return api_key

api_key = parse_api_key()

def parse_coingecko_api_key():
    """
    Parse the COINGECKO_API_KEY environment variable and return it

    If the environment variable is not set, the default API key is ""
    """
    coingecko_api_key = os.getenv('COINGECKO_API_KEY', None)
    
    return coingecko_api_key

coingecko_api_key = parse_coingecko_api_key()

SERVICE_SQS_STAGE = SQSService(SQS_STAGE, api_key)
SERVICE_SQS_PROD = SQSService(SQS_PROD, api_key)
SERVICE_SQS_LOCAL = SQSService(SQS_LOCAL, api_key)
SERVICE_COINGECKO = CoingeckoService(coingecko_api_key)
SERVICE_ASSET_LIST = AssetListService()

STAGE_INPUT_NAME = "stage"
PROD_INPUT_NAME = "prod"
LOCAL_INPUT_NAME = "local"

# Defines the mapping between the environment input name and the SQS URL.
# E.g. stage -> SQS_STAGE
INPUT_MAP = {
    STAGE_INPUT_NAME: SQS_STAGE,
    PROD_INPUT_NAME: SQS_PROD,
    LOCAL_INPUT_NAME: SQS_LOCAL
}

def parse_environments():
    """
    Parse the SQS_ENVIRONMENTS environment variable and return the corresponding SQS URLs

    If the environment variable is not set, the default environment is STAGE_INPUT_NAME
    """
    SQS_ENVIRONMENTS = os.getenv('SQS_ENVIRONMENTS', STAGE_INPUT_NAME)

    environments = SQS_ENVIRONMENTS.split(",")
    environment_urls = []
    for environment in environments:
        environment_url = INPUT_MAP.get(environment)
        if environment_url is None:
            raise Exception(f"Invalid environment: {environment}")

        environment_urls.append(environment_url)
    
    return environment_urls

# Define the environment URLs
# All tests will be run against these URLs
@pytest.fixture(params=parse_environments())

def environment_url(request):
    return request.param

SERVICE_MAP = {
    SQS_STAGE: SERVICE_SQS_STAGE,
    SQS_PROD: SERVICE_SQS_PROD,
    SQS_LOCAL: SERVICE_SQS_LOCAL
}

CHAIN_SERVICE = ChainService()

# Numia pool type constants using an Enum
class NumiaPoolType(Enum):
    BALANCER = 'osmosis.gamm.v1beta1.Pool'
    STABLESWAP = 'osmosis.gamm.poolmodels.stableswap.v1beta1.Pool'
    CONCENTRATED = 'osmosis.concentratedliquidity.v1beta1.Pool'
    COSMWASM = 'osmosis.cosmwasmpool.v1beta1.CosmWasmPool'

# Cosmwasm pool code IDs using standard constants
TRANSMUTER_CODE_ID = 148
ASTROPORT_CODE_ID = 773
ORDERBOOK_CODE_ID = 885

# Local e2e pool types using an IntEnum for convenience
class E2EPoolType(IntEnum):
    BALANCER = 0
    STABLESWAP = 1
    CONCENTRATED = 2
    COSMWASM_MISC = 3
    COSMWASM_TRANSMUTER_V1 = 4
    COSMWASM_ASTROPORT = 5
    COSMWASM_ORDERBOOK = 6

# Mapping from Numia pool types to e2e pool types
NUMIA_TO_E2E_MAP = {
    NumiaPoolType.BALANCER.value: E2EPoolType.BALANCER,
    NumiaPoolType.STABLESWAP.value: E2EPoolType.STABLESWAP,
    NumiaPoolType.CONCENTRATED.value: E2EPoolType.CONCENTRATED
}

# This is the number of tokens we allow being skipped due to being unlisted
# or not having liquidity. This number is hand-picked arbitrarily. We have around ~350 tokens
# at the time of writing this test and we leave a small buffer.
ALLOWED_NUM_TOKENS_USDC_PAIR_SKIPPED = 75
# This is the minimum number of misc token pairs
# that we expect to construct in setup.
# Since our test setup is dynamic, it is important to
# validate we do not get false positve passes due to small
# number of pairs constructed.
MIN_NUM_MISC_TOKEN_PAIRS = 10

# The file lock to ensure only one process interacts with the shared state file
sqs_e2e_data_lock_file = "/tmp/sqs_e2e_setup_data.lock"
# The shared state file to store the setup data
sqs_e2e_shared_test_state_file = "/tmp/sqs_e2e_shared_test_state.txt"

# Avoids these tokens from being chosen for testing.
token_blacklist = [
    # This is COSMO tokens that has only 2 pools.
    # One with 2K of liquidity and another one with $8M. Seems that the larged one is inflated
    # for dumping purposes. Should be fine to not route through this.
    "ibc/4925733868E7999F5822C961ADE9470A7FC5FA4A560BAE1DE102783C3F64C201"
]

# Avoids these pools from being chosen for testing.
pool_blacklist = [
    # The two pools below are likely failing pool liqidity cap check due to the wrong Numia prices.
    # Keep failing non-deterministically. Disabling for now to avoid flakiness. If larger suspicion with wrong
    # pool liquidity cap occurs, check these pools.
    817,
    1314
]

# SharedTestState class to store all the setup data
# If run in parallel mode, we generate this once from master process, write it to file
# and read it in worker processes for detereminism
# See tests/README.md for details.
class SharedTestState:
    def __init__(self, **kwargs):
        self.all_tokens_data = kwargs.get('all_tokens_data', None)
        self.all_pools_data = kwargs.get('all_pools_data', None)
        self.display_to_data_map = kwargs.get('display_to_data_map', None)
        self.chain_denom_to_data_map = kwargs.get('chain_denom_to_data_map', None)
        self.pool_type_to_denoms = kwargs.get('pool_type_to_denoms', None)
        self.denom_top_liquidity_pool_map = kwargs.get('denom_top_liquidity_pool_map', None)
        self.pool_by_id_map = kwargs.get('pool_by_id_map', None)
        self.valid_listed_tokens = kwargs.get('valid_listed_tokens', None)
        self.transmuter_token_pairs = kwargs.get('transmuter_token_pairs', None)
        self.astroport_token_pair = kwargs.get('astroport_token_pair', None)
        self.orderbook_token_pair = kwargs.get('orderbook_token_pair', None)
        self.misc_token_pairs = kwargs.get('misc_token_pairs', None)

    def to_json(self):
            # Dynamically create a dictionary of all attributes for serialization
            return {attr: getattr(self, attr) for attr in dir(self) if not attr.startswith('__') and not callable(getattr(self, attr))}

global shared_test_state
shared_test_state = SharedTestState()

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
        elif pool_code_id == ORDERBOOK_CODE_ID:
            return E2EPoolType.COSMWASM_ORDERBOOK
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
    """Return deep copy of all tokens from shared_test_state."""
    return copy.deepcopy(shared_test_state.all_tokens_data)


def choose_tokens_generic(tokens, filter_key, min_value, max_value, sort_key, num_tokens=1, asc=False, exponent_filter=None):
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
        exponent_filter (int): The exponent to filter by. None by default, signifying no filter.

    Returns:
        list: A list of denoms matching the given criteria.
    """
    # Filter tokens based on the specified filter_key range
    filtered_tokens = [
        t['denom'] for t in tokens if filter_key in t and t[filter_key] is not None and min_value <= t[filter_key] <= max_value and (exponent_filter is None or t['exponent'] == exponent_filter and t['denom'] not in token_blacklist)
    ]

    # Sort tokens based on the specified sort_key
    sorted_tokens = sorted(
        filtered_tokens, key=lambda x: next(t[sort_key] for t in tokens if t['denom'] == x), reverse=not asc
    )

    return sorted_tokens[:num_tokens]


def choose_tokens_liq_range(num_tokens=1, min_liq=0, max_liq=float('inf'), asc=False, exponent_filter=None):
    """Function to choose tokens based on liquidity."""
    tokens = get_token_data_copy()
    return choose_tokens_generic(tokens, 'liquidity', min_liq, max_liq, 'liquidity', num_tokens, asc, exponent_filter)


def choose_tokens_volume_range(num_tokens=1, min_vol=0, max_vol=float('inf'), asc=False):
    """Function to choose tokens based on volume."""
    tokens = get_token_data_copy()
    return choose_tokens_generic(tokens, 'volume_24h', min_vol, max_vol, 'volume_24h', num_tokens, asc)


def choose_pool_type_tokens_by_liq_asc(pool_type_to_denoms, pool_type, num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
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
    pools_tokens_of_type =  pool_type_to_denoms.get(pool_type, [])

    # Filter pools based on the provided min_liq and max_liq values
    filtered_pools = [
        pool for pool in pools_tokens_of_type if min_liq <= pool[1] <= max_liq
    ]

    # Sort the filtered pools based on liquidity
    sorted_pools = sorted(filtered_pools, key=lambda x: x[1], reverse=not asc)

    # Extract only the required number of pairs
    return [[pool_data[0], pool_data[2]] for pool_data in sorted_pools[:num_pairs]]


def choose_transmuter_pool_tokens_by_liq_asc(pool_type_to_denoms, num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a transmuter V1 pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(pool_type_to_denoms, E2EPoolType.COSMWASM_TRANSMUTER_V1, num_pairs, min_liq, max_liq, asc)


def choose_pcl_pool_tokens_by_liq_asc(pool_type_to_denoms, num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a Astroport PCL pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(pool_type_to_denoms, E2EPoolType.COSMWASM_ASTROPORT, num_pairs, min_liq, max_liq, asc)

def choose_orderbook_pool_tokens_by_liq_asc(pool_type_to_denoms, num_pairs=1, min_liq=0, max_liq=float('inf'), asc=False):
    """Function to choose pool ID and tokens associated with a CosmWasm orderbook pool type based on liquidity.
    Returns [pool ID, [tokens]]"""
    return choose_pool_type_tokens_by_liq_asc(pool_type_to_denoms, E2EPoolType.COSMWASM_ORDERBOOK, num_pairs, min_liq, max_liq, asc)

def choose_valid_listed_tokens(denom_top_liquidity_pool_map):
    """
    Returns all listed tokens from the asset list that have at least one pool with liquidity.

    Queries production SQS for the tokens asset list metadata.

    Queries Numia for the pool liquidity data.
    """

    # We rely on SQS itself for getting the tokens metadata for configuring tests.
    # While it is not the best practice, we make an exception since this is the most reliable way to get
    # The asset list data. In the future, we can implement custom test parsing to replace relying on SQS
    # in test setup.
    tokens_metadata = SERVICE_SQS_PROD.get_tokens_metadata()

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


def create_misc_token_pairs():
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

    tokens_metadata = SERVICE_SQS_PROD.get_tokens_metadata()

    # Filter tokens based on the 'preview' attribute in tokens_metadata
    # That is, skip unlisted tokens to avoid flakiness
    filtered_tokens = {token for token in all_tokens
                    if token in tokens_metadata and not tokens_metadata[token].get('preview', True)}

    # Construct all unique combinations of token pairs
    token_pairs = create_no_dupl_token_pairs(filtered_tokens)

    # Format pairs for return
    formatted_pairs = [[token1, token2] for token1, token2 in token_pairs]

    if len(formatted_pairs) > MIN_NUM_MISC_TOKEN_PAIRS:
        ValueError(f"Constructeed {len(formatted_pairs)}, min expected {MIN_NUM_MISC_TOKEN_PAIRS}")

    return formatted_pairs

def create_no_dupl_token_pairs(token_list):
    """
    Creates all unique combinations of token pairs from a list of tokens.


    """

    combinations = list(itertools.combinations(token_list, 2))

    filtered_list = []
    for combo in combinations:
        if combo[0] != combo[1]:
            filtered_list.append(combo)

    return filtered_list

def create_coins_from_pairs(pairs, start_order, end_order):
    """
    Create a list of token in and out pairs from the given list of token pairs, starting and ending orders of magnitude.
    For example, start_order=10^6, end_order=10^8 will generate a sequence of 3 pseudo-random amounts in the order of magnitude of 6, 7 and 8.
    E.g. [123_456, 1_234_567, 12_345_678].

    For every pair, it takes the first token as the token in and the second token as the token out. Generates amounts from [10^start_order, 10^end_order].
   
    Then, changes the token in and token out and repeats generation.

    Adds all results to a slice and returns it.
    """

    result = []

    for pair in pairs:
        # Construct combinations with denomA as token in
        denomA = pair[0]
        coin_denom_a_combos = construct_token_in_combos([denomA], start_order, end_order)

        # Construct combinations with denomB as token in
        denomB = pair[1]
        coin_denom_b_combos = construct_token_in_combos([denomB], start_order, end_order)

        # Add all combinations to the result
        for coin in coin_denom_a_combos:
            result.append({
                "token_in": coin,
                "out_denom": denomB,
            })
 
        for coin in coin_denom_b_combos:
            result.append({
                "token_in": coin,
                "out_denom": denomA,
            })

    return result

def get_usd_price_scaled(denom):
    """
    Returns the USD price of a token from the test data scaled by its precision.
    """
    denom_data = shared_test_state.chain_denom_to_data_map.get(denom)
    denom_precision = denom_data.get("exponent")
    return Decimal(denom_data.get("price")) * Decimal(10)**denom_precision

def get_denom_exponent(denom):
    """
    Returns the denom exponent given the denom itself.
    """
    denom_data = shared_test_state.chain_denom_to_data_map.get(denom)
    return denom_data.get("exponent")

def pytest_sessionstart(session):
    """
    This hook is called after the Session object has been created and
    before performing collection and entering the run test loop.

    This is where we perform the setup tasks for the tests.
    If the code is running on the master node, we fetch all the data (see data_service.py) once and store it in a shared state.

    If the code is running on a worker node, we perform worker-specific setup tasks by reading from a shared
    state file for determinism.

    See tests/README.md for details.
    """
    print("Session is starting. Worker ID:", getattr(session.config, 'workerinput', {}).get('workerid', 'master'))

    if conftest.SERVICE_COINGECKO.isServiceAvailable():
        print("Using Coingecko to calculate pool liquidity capitalization")
    else:
        print("Using Numia to calculate pool liquidity capitalization")

    global shared_test_state

    # Example setup logic
    if not hasattr(session.config, 'workerinput'):  # This checks if the code is running on the master node
        
        # Fetch all token data once
        shared_test_state.all_tokens_data = fetch_tokens()

        # Fetch all pools data once
        shared_test_state.all_pools_data = fetch_pools()

        # Create a map of display to token data
        shared_test_state.display_to_data_map = create_display_to_data_map(shared_test_state.all_tokens_data)

        # Create a map of chain denom to token data
        shared_test_state.chain_denom_to_data_map = create_chain_denom_to_data_map(shared_test_state.all_tokens_data)

        # Create two maps:
        # 1. A map of pool type to pool data
        # 2. A map of denom to top liquidity pool
        shared_test_state.pool_type_to_denoms, shared_test_state.denom_top_liquidity_pool_map = create_pool_data_maps(shared_test_state.all_pools_data)

        # Create a map of pool ID to pool data
        shared_test_state.pool_by_id_map = {str(pool.get('pool_id')): pool for pool in shared_test_state.all_pools_data}

        # Listed tokens that have at least one pool with liquidity
        shared_test_state.valid_listed_tokens = choose_valid_listed_tokens(shared_test_state.denom_top_liquidity_pool_map) 

        # One Transmuter token pair [[pool_id, ['denom0', 'denom1']]]
        shared_test_state.transmuter_token_pairs = choose_transmuter_pool_tokens_by_liq_asc(shared_test_state.pool_type_to_denoms, 1)

        # One Astroport token pair [[pool_id, ['denom0', 'denom1']]]
        shared_test_state.astroport_token_pair = choose_pcl_pool_tokens_by_liq_asc(shared_test_state.pool_type_to_denoms, 1)

        # One Orderbook token pair [[pool_id, ['denom0', 'denom1']]]
        shared_test_state.orderbook_token_pair = choose_orderbook_pool_tokens_by_liq_asc(shared_test_state.pool_type_to_denoms, 1)

        # Filter tokens data to only contain listed
        shared_test_state.all_tokens_data = [token for token in shared_test_state.all_tokens_data if token['denom'] in shared_test_state.valid_listed_tokens]

        shared_test_state.misc_token_pairs = create_misc_token_pairs()

        with FileLock(sqs_e2e_data_lock_file):
            with open(sqs_e2e_shared_test_state_file, "w") as file:
                shared_test_state_json = shared_test_state.to_json()
                file.write(json.dumps(shared_test_state_json))
    else:
        print("Performing worker-specific setup tasks...")

        with FileLock(sqs_e2e_data_lock_file):
            with open(sqs_e2e_shared_test_state_file, "r") as file:
                data_read_from_file = json.load(file)
                shared_test_state = SharedTestState(**data_read_from_file)
