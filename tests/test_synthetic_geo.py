import itertools

from sqs_service import *
from coingecko_service import *
import conftest 
from constants import *
from conftest import SERVICE_MAP
from util import *

from test_pools import run_pool_liquidity_cap_test, run_canonical_orderbook_test, run_pool_filters_test
from test_passthrough import run_test_portfolio_assets
from test_candidate_routes import run_candidate_routes_test
from test_router_quote_out_given_in import run_exact_in_quote_test

expected_latency_upper_bound_ms = 2000

# Synthetic monitoring geo-distributed test suite
class TestSyntheticMonitoringGeo:

    # /pools endpoint
    def test_synth_pools(self, environment_url):
        # OSMO/ATOM, OSMO/DAI, WBTC.eth.axl/WBTC, USDC (transmuter)
        pools = [1, 1066, 1422, 1212]

        for pool_id in pools:
            pool_data = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))
            run_pool_liquidity_cap_test(environment_url, pool_data)

    # /pools/canonical-orderbook endpoint
    def test_synth_canonical_orderbook(self, environment_url):
        run_canonical_orderbook_test(environment_url)

    # /pools endpoint with filters as query parameters
    def test_synth_pools_filters(self, environment_url):
        run_pool_filters_test(environment_url)

    # /passthrough/portfolio-assets endpoint
    def test_synth_passthrough_portfolio_assets(self, environment_url):
        run_test_portfolio_assets(environment_url)

    # /router/routes endpoint
    def test_synth_candidate_routes(self, environment_url):
        tokens_to_pair = [constants.USDC, constants.UOSMO]

        sqs_service = SERVICE_MAP[environment_url]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']

        token_pairs = list(itertools.combinations(tokens_to_pair, 2))

        for token_pair in token_pairs:
            run_candidate_routes_test(environment_url, token_pair[0], token_pair[1], expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)

    # /router/quote exact in
    def test_synth_router_quote_exact_in(self, environment_url):
        tokens_to_pair = [constants.USDC, constants.UOSMO]
        # TODO: make selection smarter
        default_amount_in = "1000000"

        token_pairs = list(itertools.combinations(tokens_to_pair, 2))

        for token_pair in token_pairs:
            run_exact_in_quote_test(environment_url, default_amount_in, token_pair[0], token_pair[1])
