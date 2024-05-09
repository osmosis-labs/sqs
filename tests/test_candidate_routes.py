import requests
import setup
import time

from sqs_service import *
import constants
from conftest import SERVICE_MAP

SQS_STAGE = "https://sqs.stage.osmosis.zone"

ROUTES_URL = "/router/routes"

# Test suite for the /router/routes endpoint
class TestCandidateRoutes:
    # Sanity check to ensure the test setup is correct
    # before continunig with more complex test cases.
    def test_usdc_uosmo(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        # Get max routes value from deployment config to expect the same number of candidate routes
        # to be found
        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']
        # Arbitrary choice based on performance at the time of test writing
        expected_latency_upper_bound_ms = 300

        self.run_candidate_routes_test(environment_url, constants.USDC, constants.UOSMO, expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)
        
    # Switch token in and out denoms compared to test_usdc_uosmo
    def test_uosmo_usdc(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        # Get max routes value from deployment config to expect the same number of candidate routes
        # to be found
        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']
        # Arbitrary choice based on performance at the time of test writing
        expected_latency_upper_bound_ms = 300

        self.run_candidate_routes_test(environment_url, constants.UOSMO, constants.USDC, expected_latency_upper_bound_ms, expected_min_routes=expected_num_routes, expected_max_routes=expected_num_routes)

    def test_all_tokens_usdc(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]

        config = sqs_service.get_config()
        expected_num_routes = config['Router']['MaxRoutes']
        # Arbitrary choice based on performance at the time of test writing
        expected_latency_upper_bound_ms = 500

        tokens_metadata = sqs_service.get_tokens_metadata()

        assert len(tokens_metadata) > 0, "Error: no tokens metadata found"
        for denom, metadata in tokens_metadata.items():

            # Skip unlisted tokens as they should be unsupported
            if metadata['is_unlisted']:
                continue

            # Set of denoms that we can skip
            skip_denoms = {
                # USDC.pica
                'ibc/078AD6F581E8115CDFBD8FFA29D8C71AFE250CE952AFF80040CBC64868D44AD3',
                # NIM network
                'ibc/279D69A6EF8E37456C8D2DC7A7C1C50F7A566EC4758F6DE17472A9FDE36C4426',
                # DAI.pica
                'ibc/37DFAFDA529FF7D513B0DB23E9728DF9BF73122D38D46824C78BB7F91E6A736B',
                # Furya
                'ibc/42D0FBF9DDC72D7359D309A93A6DF9F6FDEE3987EA1C5B3CDE95C06FCE183F12',
                # frxETH.pica
                'ibc/688E70EF567E5D4BA1CF4C54BAD758C288BC1A6C8B0B12979F911A2AE95E27EC',
                # MuseDAO
                'ibc/6B982170CE024689E8DD0E7555B129B488005130D4EDA426733D552D10B36D8F',
                # FURY.legacy
                'ibc/7CE5F388D661D82A0774E47B5129DA51CC7129BD1A70B5FA6BCEBB5B0A2FAEAF',
                # FRAX.pica,
                'ibc/9A8CBC029002DC5170E715F93FBF35011FFC9796371F59B1F3C3094AE1B453A9',
                # ASTRO
                'ibc/B8C608CEE08C4F30A15A7955306F2EDAF4A02BB191CABC4185C1A57FD978DA1B',
                # ASTRO.cw20
                'ibc/C25A2303FE24B922DAFFDCE377AC5A42E5EF746806D32E2ED4B610DE85C203F7',
                # bJUNO
                "ibc/C2DF5C3949CA835B221C575625991F09BAB4E48FB9C11A4EE357194F736111E3",
                # BSKT
                'ibc/CDD1E59BD5034C1B2597DD199782204EB397DB93200AA2E99C0AF3A66B2915FA',
                # USDC = No routes with itself
                constants.USDC,
                # Kava SWAP - pool 1631 TODO: understand why no route
                'ibc/D6C28E07F7343360AC41E15DDD44D79701DDCA2E0C2C41279739C8D4AE5264BC',
            }

            if denom in skip_denoms:
                continue

            self.run_candidate_routes_test(environment_url, denom, constants.USDC, expected_latency_upper_bound_ms, expected_min_routes=1, expected_max_routes=expected_num_routes)


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

        assert routes is not None, f"Error: no routes found for token in {token_in} and token out {token_out}"
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


def generate_all_token_usdc_tests(environment_url):
    sqs_service = SERVICE_MAP[environment_url]

    config = sqs_service.get_config()
    expected_num_routes = config['Router']['MaxRoutes']
    # Arbitrary choice based on performance at the time of test writing
    expected_latency_upper_bound_ms = 500

    tokens_metadata = sqs_service.get_tokens_metadata()

    assert len(tokens_metadata) > 0, "Error: no tokens metadata found"
    for denom, metadata in tokens_metadata.items():

        # Skip unlisted tokens as they should be unsupported
        if metadata['is_unlisted']:
            continue

        # Set of denoms that we can skip
        skip_denoms = {
            # USDC.pica
            'ibc/078AD6F581E8115CDFBD8FFA29D8C71AFE250CE952AFF80040CBC64868D44AD3',
            # NIM network
            'ibc/279D69A6EF8E37456C8D2DC7A7C1C50F7A566EC4758F6DE17472A9FDE36C4426',
            # DAI.pica
            'ibc/37DFAFDA529FF7D513B0DB23E9728DF9BF73122D38D46824C78BB7F91E6A736B',
            # Furya
            'ibc/42D0FBF9DDC72D7359D309A93A6DF9F6FDEE3987EA1C5B3CDE95C06FCE183F12',
            # frxETH.pica
            'ibc/688E70EF567E5D4BA1CF4C54BAD758C288BC1A6C8B0B12979F911A2AE95E27EC',
            # MuseDAO
            'ibc/6B982170CE024689E8DD0E7555B129B488005130D4EDA426733D552D10B36D8F',
            # FURY.legacy
            'ibc/7CE5F388D661D82A0774E47B5129DA51CC7129BD1A70B5FA6BCEBB5B0A2FAEAF',
            # FRAX.pica,
            'ibc/9A8CBC029002DC5170E715F93FBF35011FFC9796371F59B1F3C3094AE1B453A9',
            # ASTRO
            'ibc/B8C608CEE08C4F30A15A7955306F2EDAF4A02BB191CABC4185C1A57FD978DA1B',
            # ASTRO.cw20
            'ibc/C25A2303FE24B922DAFFDCE377AC5A42E5EF746806D32E2ED4B610DE85C203F7',
            # bJUNO
            "ibc/C2DF5C3949CA835B221C575625991F09BAB4E48FB9C11A4EE357194F736111E3",
            # BSKT
            'ibc/CDD1E59BD5034C1B2597DD199782204EB397DB93200AA2E99C0AF3A66B2915FA',
            # USDC = No routes with itself
            constants.USDC,
            # Kava SWAP - pool 1631 TODO: understand why no route
            'ibc/D6C28E07F7343360AC41E15DDD44D79701DDCA2E0C2C41279739C8D4AE5264BC',
        }

        if denom in skip_denoms:
            continue