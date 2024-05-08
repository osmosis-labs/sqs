import requests
import setup
import time

from sqs_service import *
import constants

SQS_STAGE = "https://sqs.stage.osmosis.zone"

ROUTES_URL = "/router/routes"

# Test suite for the /router/routes endpoint
class TestCandidateRoutes:
    # Sanity check to ensure the test setup is correct
    # before continunig with more complex test cases.
    def test_usdc_uosmo(self, environment_url):
        # Defined by the config.json.
        # TODO: read max-routes config from /config endpoint to get this value
        expected_num_routes = 20
        expected_latency_upper_bound_ms = 300

        self.run_candidate_routes_test(environment_url, constants.USDC, constants.UOSMO, expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)
        
    # Switch token in and out denoms compared to test_usdc_uosmo
    def test_uosmo_usdc(self, environment_url):
        # Defined by the config.json.
        # TODO: read max-routes config from /config endpoint to get this value
        expected_num_routes = 20
        expected_latency_upper_bound_ms = 300

        self.run_candidate_routes_test(environment_url, constants.UOSMO, constants.USDC, expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)

    def run_candidate_routes_test(self, environment_url, token_in, token_out, expected_latency_upper_bound_ms, expected_min_routes, expected_max_routes):
        """
        Runs a test for the /router/routes endpoint with the given input parameters.

        Returns routes for additional validation if needed by client

        Validates:
        - The number of routes returned
        - Following pools in each route, all tokens within these pools are present and valid
        - The latency is under the given bound
        """
        
        sqs_service = SQSService(environment_url)

        start_time = time.time()
        response = sqs_service.get_candidate_routes(token_in, token_out)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == 200, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, token in {token_in} and token out {token_out}" 

        response_json = response.json()
        routes = response_json['Routes']

        self.validate_candidate_routes(routes, token_in, token_out, expected_min_routes, expected_max_routes)

        # Return routes in case additional validation is desired
        return routes

    def validate_candidate_routes(self, routes, token_in, token_out, expected_min_routes, expected_max_routes):
        """
        Validates the given routes.

        Validates:
        - Following pools in each route, all tokens within these pools are present and valid
        - The number of routes is within the expected range
        """
        assert len(routes) <= expected_max_routes, f"Error: found more than {expected_max_routes} routes with token in {token_in} and token out {token_out}"
        assert len(routes) >= expected_min_routes, f"Error: found fewer than {expected_min_routes} routes with token in {token_in} and token out {token_out}"

        for route in routes:
            cur_token_in = token_in

            pools = route['Pools']

            assert len(pools) > 0, f"Error: no pools found in route {route}"
            for pool in pools:
                pool_id = pool['ID']

                expected_pool_data = setup.pool_by_id_map.get(pool_id)

                assert expected_pool_data, f"Error: pool ID {pool_id} not found in test data"

                # Extract denoms using a helper function
                pool_tokens = expected_pool_data.get("pool_tokens")
                denoms = setup.get_denoms_from_pool_tokens(pool_tokens)

                found_denom = cur_token_in in denoms

                assert found_denom, f"Error: token in {cur_token_in} not found in pool denoms {denoms}"

                cur_token_out = pool['TokenOutDenom']

                cur_token_in = cur_token_out

            # Last token in must be equal to token out
            assert cur_token_in == token_out, f"Error: last token out {cur_token_in} not equal to token out {token_out}"
