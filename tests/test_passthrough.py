import time
import conftest
import pytest

from sqs_service import *
from active_orderbook_orders_response import OrderbookActiveOrdersResponse
from conftest import SERVICE_MAP
from e2e_math import *
from decimal import *

# Arbitrary choice based on performance at the time of test writing
EXPECTED_LATENCY_UPPER_BOUND_MS = 1500

user_balances_assets_category_name = "user-balances"
unstaking_assets_category_name = "unstaking"
staked_assets_category_name = "staked"
inLocks_assets_category_name = "in-locks"
pooled_assets_category_name = "pooled"
unclaimed_rewards_assets_category_name = "unclaimed-rewards"
total_assets_category_name = "total-assets"

# Test suite for the /passthrough endpoint

# Note: this is for convinience to skip long-running tests in development
# locally.
# @pytest.mark.skip(reason="This test is currently disabled")


class TestPassthrough:
    def test_poortfolio_assets(self, environment_url):
        run_test_portfolio_assets(environment_url)

    def test_active_orderbook_orders(self, environment_url):
        run_test_active_orderbook_orders(environment_url)


def run_test_active_orderbook_orders(environment_url):
    sqs_service = SERVICE_MAP[environment_url]

    # list of burner addresses for the integration tests
    addresses = [
        "osmo1jgz4xmaw9yk9pjxd4h8c2zs0r0vmgyn88s8t6l",
    ]

    for address in addresses:

        start_time = time.time()
        response = sqs_service.get_active_orderbook_orders(address)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert EXPECTED_LATENCY_UPPER_BOUND_MS > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {EXPECTED_LATENCY_UPPER_BOUND_MS} ms"

        resp = OrderbookActiveOrdersResponse(**response)

        resp.validate(address)


def run_test_portfolio_assets(environment_url):
    sqs_service = SERVICE_MAP[environment_url]

    # Arbitrary addresses
    addresses = [
        "osmo1044qatzg4a0wm63jchrfdnn2u8nwdgxxt6e524",
        "osmo1aaa9rpq2m6tu6t0dvknqq2ps7zudxv7th209q4",
        "osmo18sd2ujv24ual9c9pshtxys6j8knh6xaek9z83t",
        "osmo140p7pef5hlkewuuramngaf5j6s8dlynth5zm06",
    ]

    for address in addresses:
        response = sqs_service.get_portfolio_assets(address)

        categories = response.get('categories')
        assert categories is not None

        user_balances = categories.get(user_balances_assets_category_name)
        validate_category(user_balances, True)

        unstaking = categories.get(unstaking_assets_category_name)
        validate_category(unstaking)

        staked = categories.get(staked_assets_category_name)
        validate_category(staked)

        inLocks = categories.get(inLocks_assets_category_name)
        validate_category(inLocks)

        pooled = categories.get(pooled_assets_category_name)
        validate_category(pooled)

        unclaimed_rewards = categories.get(unclaimed_rewards_assets_category_name)
        validate_category(unclaimed_rewards)

        total_assets = categories.get(total_assets_category_name)
        validate_category(total_assets, True)


def validate_category(category, should_have_breakdown=False):
    assert category is not None

    capitalization = category.get('capitalization')
    assert capitalization is not None

    assert Decimal(capitalization) >= 0

    is_best_effort = category.get('is_best_effort')
    assert not is_best_effort

    if not should_have_breakdown:
        return

    account_coins_result = category.get('account_coins_result')
    assert account_coins_result is not None

    for coin_result in account_coins_result:
        assert coin_result.get('coin') is not None
        assert coin_result.get('cap_value') is not None
