import time
import pytest

import conftest
from sqs_service import *
from quote_response import *
from rand_util import *
from e2e_math import *
from decimal import *
from constants import *
from util import *

SQS_STAGE = "https://sqs.stage.osmosis.zone"

ROUTES_URL = "/router/quote"

NUM_TOP_LIQUIDITY_DENOMS = 20

# Arbitrary choice based on performance at the time of test writing
expected_latency_upper_bound_ms = 1000

# Test suite for the /router/quote endpoint
class TestQuote:

    @pytest.mark.parametrize("coin_obj", construct_token_in_combos(conftest.choose_tokens_liq_range(NUM_TOP_LIQUIDITY_DENOMS), USDC_PRECISION - 1, USDC_PRECISION + 4), ids=id_from_coin)
    def test_usdc_in_high_liq_out(self, environment_url, coin_obj):
        """
        This test case validates quotes betwen USDC in and NUM_TOP_LIQUIDITY_DENOMS.
        The amounts are constructed to be seeded random values between 10^USDC_PRECISION-1 and 10 ^(USDC_PRECISION + 4)

        This allows us to validate that we can continue to quote at reasonable USDC values for all majore token pairs without errors.

        Note: the reason we use Decimal in this test is because floats truncate in some edge cases, leading
        to flakiness.
        """
        # This is the max error tolerance of 5% that we allow.
        error_tolerance = 0.05

        denom_out = coin_obj["denom"]
        amount_str = coin_obj["amount_str"]

        # Skip USDC quotes
        if denom_out == USDC:
            return

        denom_out_data = conftest.shared_test_state.chain_denom_to_data_map.get(denom_out)
        denom_out_precision = denom_out_data.get("exponent")
        
        # Compute spot price scaling factor.
        spot_price_scaling_factor = Decimal(10)**6 / Decimal(10)**denom_out_precision

        # Compute expected spot prices
        out_base_in_quote_price = Decimal(denom_out_data.get("price"))
        expected_in_base_out_quote_price = 1 / out_base_in_quote_price
        
        # Compute expected token out
        expected_token_out = int(amount_str) / out_base_in_quote_price

        # Set the token in coin
        token_in_coin = amount_str + USDC

        # Run the quote test
        quote = self.run_quote_test(environment_url, token_in_coin, denom_out, expected_latency_upper_bound_ms)

        # Validate routes are generally present
        assert len(quote.route) > 0

        # Check if the route is a single pool single transmuter route
        # For such routes, the price impact is 0.
        is_transmuter_route = self.is_transmuter_in_single_route(quote.route)

        # Validate price impact
        # If it is a single pool single transmuter route, we expect the price impact to be 0
        # Price impact is returned as a negative number for any other route.
        assert quote.price_impact is not None
        assert (not is_transmuter_route) and (quote.price_impact < 0) or (is_transmuter_route) and (quote.price_impact == 0), f"Error: price impact {quote.price_impact} is zero for non-transmuter route"
        price_impact_positive = quote.price_impact * -1

        # Validate amount in and denom are as input
        assert quote.amount_in.amount == int(amount_str)
        assert quote.amount_in.denom == USDC

        # Validate that the fee is charged
        assert quote.effective_fee > 0

        # Validate that the spot price is present
        assert quote.in_base_out_quote_spot_price is not None

        # Validate that the spot price is within the error tolerance
        assert relative_error(quote.in_base_out_quote_spot_price * spot_price_scaling_factor, expected_in_base_out_quote_price) < error_tolerance, f"Error: in base out quote spot price {quote.in_base_out_quote_spot_price} is not within {error_tolerance} of expected {expected_in_base_out_quote_price}"

        # If there is a price impact greater than the provided error tolerance, we dynamically set the error tolerance to be
        # the price impact * (1 + error_tolerance) to account for the price impact
        if price_impact_positive > error_tolerance:
            error_tolerance = price_impact_positive * Decimal(1 + error_tolerance)

        # Validate that the amount out is within the error tolerance
        assert relative_error(quote.amount_out * spot_price_scaling_factor, expected_token_out) < error_tolerance, f"Error: amount out {quote.amount_out} is not within {error_tolerance} of expected {expected_token_out}"

    # Test various combinations between tokens in the following groups:
    # Selects the following groups of tokens:
    # 1. Top 5 by-liquidity
    # 2. Top 5 by-volume
    # 3. Five low liquidity (between 5000 and 10000 USD)
    # 4. Five low volume (between 5000 and 10000 USD)
    @pytest.mark.parametrize("swap_pair", conftest.create_coins_from_pairs(conftest.shared_test_state.misc_token_pairs, 6, 9), ids=id_from_swap_pair)
    def test_misc_token_pairs(self, environment_url, swap_pair):
       pass

    def run_quote_test(self, environment_url, token_in, token_out, expected_latency_upper_bound_ms, expected_status_code=200) -> QuoteResponse:
        """
        Runs a test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency

        Returns quote for additional validation if needed by client

        Validates:
        - Response status code is as given or default 200
        - Latency is under the given bound
        """
        
        sqs_service = conftest.SERVICE_MAP[environment_url]

        start_time = time.time()
        response = sqs_service.get_quote(token_in, token_out)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == expected_status_code, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, token in {token_in} and token out {token_out}" 

        response_json = response.json()

        # Return route for more detailed validation
        return QuoteResponse(**response_json)

    def is_transmuter_in_single_route(self, routes):
        """
        Returns true if there is a single route with
        one transmuter pool in it.
        """
        if len(routes) == 1 and len(routes[0].pools) == 1:
            pool_in_route = routes[0].pools[0]
            pool = conftest.shared_test_state.pool_by_id_map.get(pool_in_route.id)
            e2e_pool_type = conftest.get_e2e_pool_type_from_numia_pool(pool)

            return  e2e_pool_type == conftest.E2EPoolType.COSMWASM_TRANSMUTER_V1
        
        return False
