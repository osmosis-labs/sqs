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
    @pytest.mark.parametrize("pair", conftest.create_coins_from_pairs(conftest.create_no_dupl_token_pairs(conftest.choose_tokens_liq_range(num_tokens=10, min_liq=500_000, exponent_filter=USDC_PRECISION)), USDC_PRECISION, USDC_PRECISION + 3), ids=id_from_swap_pair)
    def test_get_custom_direct_quote(self, environment_url, pair):
        denom_in = pair['out_denom']
        coin = Coin(pair['token_in']['denom'], pair['token_in']['amount_str'])
        token_out  = pair['token_in']['amount_str'] + pair['token_in']['denom']

        # Get the optimal quote for the given token pair
        optimal_quote = ExactAmountOutQuote.run_quote_test(environment_url, token_out, denom_in, EXPECTED_LATENCY_UPPER_BOUND_MS)

        
        # Construct input parameters for the custom direct quote
        pools = []
        denoms = []
        for route in optimal_quote.route:
            for pool in route.pools:
                denoms.append(pool.token_in_denom)
                pools.append(str(pool.id))
        pool_id = ','.join(pools)
        denoms_in = ','.join(denoms)

        token_out = "3254257uosmo"
        denoms_in = "ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4,ibc/D79E7D83AB399BFFF93433E54FAA480C191248FC556924A2A8351AE2638B3877"
        pool_id = "1263,1247"

        quote = self.run_quote_test(environment_url, token_out, denoms_in, pool_id, EXPECTED_LATENCY_UPPER_BOUND_MS)
        # quote = self.run_quote_test(environment_url, token_out, denoms_in, pool_id, EXPECTED_LATENCY_UPPER_BOUND_MS)
        print(pool_id)

        # All tokens have the same default exponent, resulting in scaling factor of 1.
        spot_price_scaling_factor = 1

        # Compute expected base out quote spot price
        # First, get the USD price of each denom, and then divide to get the expected spot price
        in_base_usd_quote_price = conftest.get_usd_price_scaled(denom_in)
        out_base_usd_quote_price = conftest.get_usd_price_scaled(coin.denom)
        expected_in_base_out_quote_price = out_base_usd_quote_price / in_base_usd_quote_price 

        # TODO: fix this
        quote.price_impact = -1

        # Compute expected token out
        expected_token_in = int(coin.amount) * expected_in_base_out_quote_price

        token_out_amount_usdc_value = in_base_usd_quote_price * coin.amount

        # Chose the error tolerance based on amount in swapped.
        error_tolerance = Quote.choose_error_tolerance(token_out_amount_usdc_value)

        # Validate that price impact is present.
        assert quote.price_impact is not None

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
        
        print(response.text)

        # Return route for more detailed validation
        return QuoteExactAmountOutResponse(**response_json)

