import conftest
import pytest

from sqs_service import *
import util
from conftest import SERVICE_MAP
from e2e_math import *

# Test suite for the /pools endpoint

# Note: this is for convinience to skip long-running tests in development
# locally.
# @pytest.mark.skip(reason="This test is currently disabled")
class TestPools:
    # Test all valid pools as given by Numia
    @pytest.mark.parametrize("pool_data", conftest.shared_test_state.all_pools_data, ids=util.id_from_pool)
    def test_pools_pool_liquidity_cap(self, environment_url, pool_data):
        # Relative errorr tolerance for pool liquidity cap
        error_tolerance = 0.05
        # Min liquidity capitalization in USDC for a pool to be considered
        # in tests.
        min_pool_liquidity_cap_usdc = 50_000
        # WhiteWhale pools are not supported by Numia, leading to breakages.
        # See: https://linear.app/osmosis/issue/NUMIA-35/missing-data-for-white-whale-pool
        skip_whitewhale_code_id = 641

        sqs_service = SERVICE_MAP[environment_url]

        pool_liquidity = pool_data.get("liquidity")
        pool_id = pool_data.get("pool_id")

        sqs_pool = sqs_service.get_pool(pool_id)

        # Skip white whale pool since it has flakiness on Numia side
        code_id = pool_data.get("code_id")
        if code_id is not None and int(code_id) == skip_whitewhale_code_id:
            pytest.skip("Skipping white whale pool since it has flakiness on Numia side")

        # Skip pool if liquidity is too low
        if pool_liquidity > min_pool_liquidity_cap_usdc:
            sqs_liquidity_cap = int(sqs_pool[0].get("liquidity_cap"))

            actual_error = relative_error(sqs_liquidity_cap, pool_liquidity)

            assert actual_error < error_tolerance, f"Pool liquidity cap was {sqs_liquidity_cap} - expected {pool_liquidity}, actual error {actual_error} error tolerance {error_tolerance}" 
        else:
            pytest.skip("Pool liquidity is too low - skipping to reduce flakiness")

