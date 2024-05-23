import conftest
import time
import pytest

from sqs_service import *
import constants
from conftest import SERVICE_MAP

# Arbitrary choice based on performance at the time of test writing
expected_latency_upper_bound_ms = 1000

# Test suite for the /router/routes endpoint

# Note: this is for convinience to skip long-running tests in development
# locally.
# @pytest.mark.skip(reason="This test is currently disabled")
class TestCandidateRoutes:
    # Sanity check to ensure the test setup is correct
    # before continunig with more complex test cases.
    def test_usdc_uosmo(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']

        self.run_candidate_routes_test(environment_url, constants.USDC, constants.UOSMO, expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)
        
    # Switch token in and out denoms compared to test_usdc_uosmo
    def test_uosmo_usdc(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']

        self.run_candidate_routes_test(environment_url, constants.UOSMO, constants.USDC, expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)

    # Test all valid listed tokens with appropriate liquidity with dynamic parameterization
    @pytest.mark.parametrize("denom", conftest.shared_test_state.valid_listed_tokens)
    def test_all_valid_tokens(self, environment_url, denom):
        sqs_service = SERVICE_MAP[environment_url]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']


        self.run_candidate_routes_test(environment_url, denom, constants.USDC, expected_latency_upper_bound_ms, expected_min_routes=1, expected_max_routes=expected_num_routes)

    def test_transmuter_tokens(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        transmuter_token_data = conftest.shared_test_state.transmuter_token_pairs[0]
        transmuter_pool_id = transmuter_token_data[0]
        tansmuter_token_pair = transmuter_token_data[1]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']

        routes = self.run_candidate_routes_test(environment_url, tansmuter_token_pair[0], tansmuter_token_pair[1], expected_latency_upper_bound_ms, expected_min_routes=1, expected_max_routes=expected_num_routes)

        validate_pool_id_in_route(routes, [transmuter_pool_id])
    
    def test_astroport_tokens(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        astroport_token_data = conftest.shared_test_state.astroport_token_pair[0]
        astroport_pool_id = astroport_token_data[0]
        astroport_token_pair = astroport_token_data[1]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']

        routes = self.run_candidate_routes_test(environment_url, astroport_token_pair[0], astroport_token_pair[1], expected_latency_upper_bound_ms, expected_min_routes=1, expected_max_routes=expected_num_routes)

        validate_pool_id_in_route(routes, [astroport_pool_id])

    # Test various combinations between tokens in the following groups:
    # Selects the following groups of tokens:
    # 1. Top 5 by-liquidity
    # 2. Top 5 by-volume
    # 3. Five low liquidity (between 5000 and 10000 USD)
    # 4. Five low volume (between 5000 and 10000 USD)
    @pytest.mark.parametrize("pair", conftest.shared_test_state.misc_token_pairs)
    def test_misc_token_pairs(self, environment_url, pair):
        sqs_service = SERVICE_MAP[environment_url]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']

        self.run_candidate_routes_test(environment_url, pair[0], pair[1], expected_latency_upper_bound_ms, expected_min_routes=1, expected_max_routes=expected_num_routes)

    def run_candidate_routes_test(self, environment_url, token_in, token_out, expected_latency_upper_bound_ms, expected_min_routes, expected_max_routes):
        """
        Runs a test for the /router/routes endpoint with the given input parameters.

        Returns routes for additional validation if needed by client

        Validates:
        - The number of routes returned
        - Following pools in each route, all tokens within these pools are present and valid
        - The latency is under the given bound
        """
        
        sqs_service = SERVICE_MAP[environment_url]

        start_time = time.time()
        response = sqs_service.get_candidate_routes(token_in, token_out)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == 200, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, token in {token_in} and token out {token_out}" 

        response_json = response.json()
        routes = response_json['Routes']

        validate_candidate_routes(routes, token_in, token_out, expected_min_routes, expected_max_routes)

        # Return routes in case additional validation is desired
        return routes

def validate_candidate_routes(routes, token_in, token_out, expected_min_routes, expected_max_routes):
    """
    Validates the given routes.

    Validates:
    - Following pools in each route, all tokens within these pools are present and valid
    - The number of routes is within the expected range
    """

    if token_in == token_out:
        assert routes is None, f"equal tokens in and out for candidate route must have no route"
        return

    assert routes is not None, f"Error: no routes found for token in {token_in} and token out {token_out}"
    assert len(routes) <= expected_max_routes, f"Error: found more than {expected_max_routes} routes with token in {token_in} and token out {token_out}"
    assert len(routes) >= expected_min_routes, f"Error: found fewer than {expected_min_routes} routes with token in {token_in} and token out {token_out}"

    for route in routes:
        cur_token_in = token_in

        pools = route['Pools']

        assert len(pools) > 0, f"Error: no pools found in route {route}"
        for pool in pools:
            pool_id = pool['ID']

            expected_pool_data = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))

            assert expected_pool_data, f"Error: pool ID {pool_id} not found in test data"

            # Extract denoms using a helper function
            pool_tokens = expected_pool_data.get("pool_tokens")
            denoms = conftest.get_denoms_from_pool_tokens(pool_tokens)

            found_denom = cur_token_in in denoms

            assert found_denom, f"Error: token in {cur_token_in} not found in pool denoms {denoms}"

            cur_token_out = pool['TokenOutDenom']

            cur_token_in = cur_token_out

        # Last token in must be equal to token out
        assert cur_token_in == token_out, f"Error: last token out {cur_token_in} not equal to token out {token_out}"

def validate_pool_id_in_route(routes, expected_pool_ids):
    """
    Validates that there is at least one route in routes
    that contains pools exactly as given per expected_pool_ids

    Fails if not
    """

    assert len(routes) > 0
    for route in routes:
        pools = route['Pools']

        if len(pools) != len(expected_pool_ids):
            continue

        is_expected_route = True
        for pool, expected_pool_id in zip(pools, expected_pool_ids):
            pool_id = pool['ID']

            if pool_id != expected_pool_id:
                is_expected_route = False
                break
        
        if is_expected_route:
            return

    assert False, f"{routes} do not contain {expected_pool_id}"
