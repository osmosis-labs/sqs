import conftest
import pytest

from sqs_service import *
import util
from conftest import SERVICE_MAP
from e2e_math import *

# Test suite for the /pools endpoint

# Min liquidity capitalization in USDC for a pool to be considered
# in tests. Arbitrarily chosen as to avoid flakiness.
min_pool_liquidity_cap_usdc = 50_000

def filter_pools(pools_data, min_pool_liquidity_cap_usdc):

    filtered_pool_data = []

    for pool_data in pools_data:
        pool_liquidity = pool_data.get("liquidity")
        if pool_liquidity > min_pool_liquidity_cap_usdc:
            filtered_pool_data.append(pool_data)

    return filtered_pool_data

# Note: this is for convinience to skip long-running tests in development
# locally.
# @pytest.mark.skip(reason="This test is currently disabled")
class TestPools:
    # Test test runs for all pools that have liquidity over min_pool_liquidity_cap_usdc as given by external
    # data service to avoid flakiness.
    # The test checks if the pool liquidity cap is within 5% of the expected value.
    # The expected value is given by the external data service.
    @pytest.mark.parametrize("pool_data", filter_pools(conftest.shared_test_state.all_pools_data, min_pool_liquidity_cap_usdc), ids=util.id_from_pool)
    def test_pools_pool_liquidity_cap(self, environment_url, pool_data):
        # Relative errorr tolerance for pool liquidity cap
        error_tolerance = 0.07

        # WhiteWhale pools are not supported by Numia, leading to breakages.
        # See: https://linear.app/osmosis/issue/NUMIA-35/missing-data-for-white-whale-pool
        skip_whitewhale_code_id = 641
        # This pool has a bug in the Numia side.
        skip_alloyed_pool_id = 1816

        sqs_service = SERVICE_MAP[environment_url]

        pool_liquidity = pool_data.get("liquidity")
        pool_id = pool_data.get("pool_id")

        if pool_id == skip_alloyed_pool_id:
            pytest.skip("Skipping alloyed pool since it has flakiness on Numia side")

        sqs_pool = sqs_service.get_pool(pool_id)

        # Skip white whale pool since it has flakiness on Numia side
        code_id = pool_data.get("code_id")
        if code_id is not None and int(code_id) == skip_whitewhale_code_id:
            pytest.skip("Skipping white whale pool since it has flakiness on Numia side")

        # Skip pool if liquidity is too low
        if pool_liquidity > min_pool_liquidity_cap_usdc:
            sqs_liquidity_cap = int(sqs_pool[0].get("liquidity_cap"))

            actual_error = relative_error(sqs_liquidity_cap, pool_liquidity)

            assert actual_error < error_tolerance, f"ID ({pool_id}) Pool liquidity cap was {sqs_liquidity_cap} - expected {pool_liquidity}, actual error {actual_error} error tolerance {error_tolerance}" 
        else:
            pytest.skip("Pool liquidity is too low - skipping to reduce flakiness")

    def test_canonical_orderbook(self, environment_url):
        # Note, that this is the first orderbook created on mainnet. As a result, it is the canonical orderbook.
        # If more orederbooks are added in the future and liquidity changes, this might have to be refactored.
        expected_orderbook_pool_id = 1904
        base = "factory/osmo1z0qrq605sjgcqpylfl4aa6s90x738j7m58wyatt0tdzflg2ha26q67k743/wbtc"
        quote = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4"
        expected_contract_address = "osmo18nzruvalfuukut9fq5st5mg5sn6s8nu4u42kuwwgynu3fne6sd5sxnrwf2"

        sqs_service = SERVICE_MAP[environment_url]
        canonical_orderbooks = sqs_service.get_canonical_orderbooks()

        assert canonical_orderbooks is not None, "Canonical orderbooks are None"
        assert len(canonical_orderbooks) > 0, "Canonical orderbooks are empty"

        didFind = False

        for orderbook in canonical_orderbooks:
            if orderbook.get("pool_id") == expected_orderbook_pool_id:
                assert orderbook.get("base") == base, "Base asset is not correct"
                assert orderbook.get("quote") == quote, "Quote asset is not correct"

                actual_contract_address = orderbook.get("contract_address")
                assert actual_contract_address == expected_contract_address, f"Contract address is not correct, actual: {actual_contract_address}, expected: {expected_contract_address}"


                didFind = True
                return

        # This may fail as we keep adding more orderbooks. If it does, we need to refactor this test.
        assert didFind, "Expected canonical orderbook was not found"
