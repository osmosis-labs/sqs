import pytest

import constants
import conftest

def id_from_coin(coin_obj):
    """
    Returns a string id from a coin object. This is useful for generating meaningful test
    IDs for parametrized tests.
    """
    if coin_obj is None:
        return "None"
    return f"{coin_obj['amount_str'] + coin_obj['denom']}"

def id_from_swap_pair(swap_pair):
    """
    Returns a string id from a swap pair object that contains token_in and out_denom. This is useful for generating meaningful test
    IDs for parametrized tests.
    """
    if swap_pair is None:
        return "None"

    token_in_obj = swap_pair['token_in']
    token_in_str = token_in_obj['amount_str'] + token_in_obj['denom']

    return f"{token_in_str + '-' + swap_pair['out_denom']}"


def skip_imbalanced_pool_test_if_imbalanced(token_data):
    """
    Skip the test if any of the tokens in the pool (token_data[0]) have less than TRANSMUTER_MIN_TOKEN_LIQ_USD liquidity.
    See definition of TRANSMUTER_MIN_TOKEN_LIQ_USD for more information.

    This is useful for skipping pools such as transmuter that tend to get out of balance, consisting
    only of one token and causing the flakiness in our test suite.
    """
    pool_id = token_data[0]
    pool_data = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))
    pool_tokens = pool_data.get("pool_tokens")
    for token in pool_tokens:
        if float(token.get("amount")) < constants.TRANSMUTER_MIN_TOKEN_LIQ_USD:
            pytest.skip("Skipped test due to pool being imbalanced")
            return
