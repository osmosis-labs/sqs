import conftest
import pytest
from decimal import Decimal

from sqs_service import *
import util
from conftest import SERVICE_MAP
from e2e_math import *

# Test suite for the /pools endpoint

# Min liquidity capitalization in USDC for a pool to be considered
# in tests. Arbitrarily chosen as to avoid flakiness.
min_pool_liquidity_cap_usdc = 100_000
num_all_pools_expected = 1500

def filter_pools(pools_data, min_pool_liquidity_cap_usdc):

    filtered_pool_data = []

    for pool_data in pools_data:

        pool_id = pool_data.get("pool_id")
        if pool_id in conftest.pool_blacklist:
            continue

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
        run_pool_liquidity_cap_test(environment_url, pool_data)

    def test_canonical_orderbook(self, environment_url):
        run_canonical_orderbook_test(environment_url)

    def test_pool_filters(self, environment_url):
        run_pool_filters_test(environment_url)

def run_pool_liquidity_cap_test(environment_url, pool_data):
    # Relative errorr tolerance for pool liquidity cap
    error_tolerance = 0.07

    # WhiteWhale pools are not supported by Numia, leading to breakages.
    # See: https://linear.app/osmosis/issue/NUMIA-35/missing-data-for-white-whale-pool
    skip_whitewhale_code_id = 641
    # This pool has a bug in the Numia side.
    skip_alloyed_pool_id = [1816, 1868, 1878, 1925]

    sqs_service = SERVICE_MAP[environment_url]

    pool_id = pool_data.get("pool_id")

    pool_liquidity = pool_data.get("liquidity")

    if pool_id in skip_alloyed_pool_id:
        pytest.skip("Skipping alloyed pool since it has flakiness on Numia side")

    # Skip white whale pool since it has flakiness on Numia side
    code_id = pool_data.get("code_id")
    if code_id is not None and int(code_id) == skip_whitewhale_code_id:
        pytest.skip("Skipping white whale pool since it has flakiness on Numia side")

    # Skip pool if liquidity is too low
    if pool_liquidity <= min_pool_liquidity_cap_usdc:
        pytest.skip("Pool liquidity is too low - skipping to reduce flakiness")

    pool_liquidity = pool_data.get("liquidity")

    sqs_pool = sqs_service.get_pools(pool_ids=pool_id)
 
    sqs_liquidity_cap = int(sqs_pool[0].get("liquidity_cap"))

    actual_error = relative_error(sqs_liquidity_cap, pool_liquidity)

    assert actual_error < error_tolerance, f"ID ({pool_id}) Pool liquidity cap was {sqs_liquidity_cap} - expected {pool_liquidity}, actual error {actual_error} error tolerance {error_tolerance}" 

def run_canonical_orderbook_test(environment_url):
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

def run_pool_filters_test(environment_url):
    """
    Test the pool filters for the /pools endpoint.
    - No filters
    - Only pool ID filter
    - Min Liquidity cap filter
    - Both pool ID and min liquidity cap filter
    """

    sqs_service = SERVICE_MAP[environment_url]

    # $100k
    min_liquidity_cap = 100_000
    #         # Pool 1 and 135 are major pools but 37 is junk.
    pool_id_filter = "1,1135,37"

    expected_num_filtered_pools = 3
    # Filter pool id 37 which s junk
    expected_num_pools_both_filters = expected_num_filtered_pools - 1

    # 50 is chosen arbitrarily low to avoid flakiness
    expected_num_min_liq_cap_pools = 50

    # Test with all filters
    all_pools = sqs_service.get_pools()
    assert all_pools is not None, "All pools are None"
    assert len(all_pools) > num_all_pools_expected, f"Number of all pools should be greater than {len(all_pools)}"


    filtered_pools = sqs_service.get_pools(pool_ids=pool_id_filter)
    assert filtered_pools is not None, "Fitlered pools are None"
    assert len(filtered_pools) == expected_num_filtered_pools, f"Number of filtered pools should be {expected_num_filtered_pools}, actual {len(filtered_pools)}"

    # Test with min liquidity cap filter
    filtered_pools = sqs_service.get_pools(min_liquidity_cap=min_liquidity_cap)
    assert filtered_pools is not None, "Fitlered pools are None"

    assert len(filtered_pools) > expected_num_min_liq_cap_pools, f"Number of filtered pools should be greater than {expected_num_min_liq_cap_pools}"

    # Test with both pool ID and min liquidity cap filter
    filtered_pools = sqs_service.get_pools(pool_ids=pool_id_filter, min_liquidity_cap=min_liquidity_cap)
    assert filtered_pools is not None, "Fitlered pools are None"

    assert len(filtered_pools) == expected_num_pools_both_filters, f"Number of filtered pools should be {expected_num_pools_both_filters}, actual {len(filtered_pools)}"

    # Test APR and fee data
    filtered_apr_fee_pools = sqs_service.get_pools(pool_ids="1135", with_market_incentives=True)
    assert filtered_apr_fee_pools is not None, "Fitlered pools are None"

    assert len(filtered_apr_fee_pools) == 1, f"Number of filtered pools should be 1, actual {len(filtered_apr_fee_pools)}"
    pool = filtered_apr_fee_pools[0]
    # assert pool.get("pool_id") == 1135, "Pool ID is not correct"

    # Validate swap fees APR data is present
    apr_data = pool.get("apr_data")
    assert apr_data is not None, "APR data is None"
    swap_fees = apr_data.get("swap_fees").get("upper")
    assert swap_fees is not None, "APR data is None"
    assert swap_fees > 0, "Swap fees should be greater than 0"

    # Validate fee volume data is present
    fee_data = pool.get("fees_data")
    assert fee_data is not None, "Fee data is None"

    volume_24h = fee_data.get("volume_24h")
    assert volume_24h is not None, "Volume 24h is None"
    assert volume_24h > 0, "Volume 24h should be greater than 0"

    volume_7d = fee_data.get("volume_7d")
    assert volume_7d is not None, "Volume 7d is None"
    assert volume_7d > 0, "Volume 7d should be greater than 0"

    fees_spent_24h = fee_data.get("fees_spent_24h")
    assert fees_spent_24h is not None, "Fees spent 24h is None"
    assert fees_spent_24h > 0, "Fees spent 24h should be greater than 0"

    fees_spent_7d = fee_data.get("fees_spent_7d")
    assert fees_spent_7d is not None, "Fees spent 7d is None"
    assert fees_spent_7d > 0, "Fees spent 7d should be greater than 0"

    fees_percentage = fee_data.get("fees_percentage")
    assert fees_percentage is not None, "Fees percentage 24h is None"
    assert fees_percentage == "0.2%", "Fees percentage should be 0.2%"
