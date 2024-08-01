import time
import pytest

import conftest
from sqs_service import *
from quote import *
from quote_response import *
from rand_util import *
from e2e_math import *
from decimal import *
from constants import *
from util import *
from route import *

ROUTES_URL = "/router/custom-direct-quote"

# Arbitrary choice based on performance at the time of test writing
EXPECTED_LATENCY_UPPER_BOUND_MS = 15000

# Test suite for the /router/custom-direct-quote endpoint
# Test runs tests for exact amount out quotes.
class TestExactAmountOutDirectCustomQuote:
    @pytest.mark.parametrize("token_out, denom_in, pool_id", [
        # TODO: multi pool, single pool
        # Valid exact amount out swap with multiple denoms and matching pool_id
        ("2353uion", "uosmo", "2"),
    ])
    def test_get_custom_direct_quote(self, environment_url, token_out, denom_in, pool_id):
        quote = self.run_quote_test(environment_url, token_out, denom_in, pool_id, EXPECTED_LATENCY_UPPER_BOUND_MS)

        # Validate that price impact is present.
        assert quote.price_impact is not None

        # TODO
        coin = Coin("uion", "2353")
        amount = ExactAmountOutQuote.calculate_amount(coin, denom_in)

        error_tolerance = Quote.choose_error_tolerance(amount)

        # Validate quote results
        ExactAmountOutQuote.validate_quote_test(quote, coin.amount, coin.denom, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_in, denom_in, error_tolerance)

    def run_quote_test(self, environment_url, token_out, denom_in, pool_id, expected_latency_upper_bound_ms, expected_status_code=200) -> QuoteExactAmountOutResponse:
        """
        Runs exact amount out test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency

        Returns quote for additional validation if needed by client

        Validates:
        - Response status code is as given or default 200
        - Latency is under the given bound
        """

        sqs_service = conftest.SERVICE_MAP[environment_url]

        start_time = time.time()
        response = sqs_service.get_exact_amount_out_custom_direct_quote(token_out, denom_in, pool_id)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == expected_status_code, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, token in {token_in} and token out {token_out}" 

        response_json = response.json()

        # Return route for more detailed validation
        return QuoteExactAmountOutResponse(**response_json)

