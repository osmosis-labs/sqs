import setup
import constants
import pytest

def skip_imbalanced_pool_test(token_data):
    """
    Skip the test if any of the tokens in the pool (token_data[0]) have less than TRANSMUTER_MIN_TOKEN_LIQ_USD liquidity.
    See definition of TRANSMUTER_MIN_TOKEN_LIQ_USD for more information.

    This is useful for skipping pools such as transmuter that tend to get out of balance, consisting
    only of one token and causing the flakiness in our test suite.
    """
    pool_id = token_data[0]
    pool_data = setup.pool_by_id_map.get(pool_id)
    pool_tokens = pool_data.get("pool_tokens")
    for token in pool_tokens:
        if float(token.get("amount")) < constants.TRANSMUTER_MIN_TOKEN_LIQ_USD:
            pytest.skip("Skipped test due to pool being imbalanced")
            return
