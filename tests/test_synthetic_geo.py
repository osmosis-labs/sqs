import pytest
import timeit
import time
import os

from datetime import datetime
from sqs_service import *
from coingecko_service import *
import conftest 
from constants import *
from conftest import SERVICE_MAP
from filelock import FileLock
from util import *

from test_pools import run_pool_liquidity_cap_test, run_canonical_orderbook_test, run_pool_filters_test
from test_passthrough import run_test_portfolio_assets

counter_file = "/tmp/counter.txt"
lock_file = "/tmp/counter.lock"

# Synthetic monitoring geo-distributed test suite
class TestSyntheticMonitoringGeo:
    def test_synth_pools(self, environment_url):
        # OSMO/ATOM, OSMO/DAI, WBTC.eth.axl/WBTC, USDC (transmuter)
        pools = [1, 1066, 1422, 1212]

        for pool_id in pools:
            pool_data = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))
            run_pool_liquidity_cap_test(environment_url, pool_data)

    def test_synth_canonical_orderbook(self, environment_url):
        run_canonical_orderbook_test(environment_url)

    def test_synth_pools_filters(self, environment_url):
        run_pool_filters_test(environment_url)

    def test_synth_passthrough_portfolio_assets(self, environment_url):
        run_test_portfolio_assets(environment_url)
