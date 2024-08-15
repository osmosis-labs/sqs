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

# Arbitrary choice based on performance at the time of test writing
EXPECTED_LATENCY_UPPER_BOUND_MS = 15000

# Test suite for the /router/custom-direct-quote endpoint
# Test runs tests for exact amount in quotes.
class TestExactAmountInDirectCustomQuote:
    @pytest.mark.parametrize("pair", conftest.create_coins_from_pairs(conftest.create_no_dupl_token_pairs(conftest.choose_tokens_liq_range(num_tokens=10, min_liq=500_000, exponent_filter=USDC_PRECISION)), USDC_PRECISION, USDC_PRECISION + 3), ids=id_from_swap_pair)
    def test_get_custom_direct_quote(self, environment_url, pair):
        denom_out = pair['out_denom']
        coin = Coin(pair['token_in']['denom'], pair['token_in']['amount_str'])
        token_in  = pair['token_in']['amount_str'] + pair['token_in']['denom']

        # Get the optimal quote for the given token pair
        # Direct custom quote does not support multiple routes, so we request single/multi hop pool routes only
        optimal_quote = ExactAmountInQuote.run_quote_test(environment_url, token_in, denom_out, False, True, EXPECTED_LATENCY_UPPER_BOUND_MS)

        pool_id = ','.join(map(str, optimal_quote.get_pool_ids()))
        denoms_out = ','.join(map(str, optimal_quote.get_token_out_denoms()))

        quote = self.run_quote_test(environment_url, token_in, denoms_out, pool_id, EXPECTED_LATENCY_UPPER_BOUND_MS)

        # All tokens have the same default exponent, resulting in scaling factor of 1.
        spot_price_scaling_factor = 1

        # Compute expected base out quote spot price
        # First, get the USD price of each denom, and then divide to get the expected spot price
        in_base_usd_quote_price = conftest.get_usd_price_scaled(denom_out)
        out_base_usd_quote_price = conftest.get_usd_price_scaled(coin.denom)
        expected_in_base_out_quote_price = out_base_usd_quote_price / in_base_usd_quote_price 

        # Compute expected token out
        expected_token_in = int(coin.amount) * expected_in_base_out_quote_price

        token_in_amount_usdc_value = in_base_usd_quote_price * coin.amount

        # Chose the error tolerance based on amount in swapped.
        error_tolerance = Quote.choose_error_tolerance(token_in_amount_usdc_value)

        # Validate that price impact is present.
        assert quote.price_impact is not None

        # Validate quote results
        ExactAmountInQuote.validate_quote_test(quote, coin.amount, coin.denom, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_in, denom_out, error_tolerance, True)

    def run_quote_test(self, environment_url, token_in, denom_out, pool_id, expected_latency_upper_bound_ms, expected_status_code=200) -> QuoteExactAmountInResponse:
        """
        Runs exact amount in test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency

        Returns quote for additional validation if needed by client

        Validates:
        - Response status code is as given or default 200
        - Latency is under the given bound
        """

        sqs_service = conftest.SERVICE_MAP[environment_url]

        start_time = time.time()
        response = sqs_service.get_exact_amount_in_custom_direct_quote(token_in, denom_out, pool_id)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == expected_status_code, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, denom in {denom_out} and token out {token_in}" 

        response_json = response.json()
        
        print(response.text)

        # Return route for more detailed validation
        return QuoteExactAmountInResponse(**response_json)

