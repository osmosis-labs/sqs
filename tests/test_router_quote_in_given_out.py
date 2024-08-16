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

ROUTES_URL = "/router/quote"

QUOTE_NUM_TOP_LIQUIDITY_DENOMS = 20

# Arbitrary choice based on performance at the time of test writing
EXPECTED_LATENCY_UPPER_BOUND_MS = 15000

# The max amount in value in USD to run the price impact check
# This is primarily to avoid flakiness due to swapping large amounts.
# The choice is arbitrary and was made based on testing at the time of creation.
# In the future, we might lower or increase this value based on the performance of the system.
HIGH_LIQ_PRICE_IMPACT_CHECK_USD_AMOUNT_IN_THRESHOLD = 5000

# The max price impact threshold for the high liquidity check
HIGH_LIQ_MAX_PRICE_IMPACT_THRESHOLD = 0.5

# Test suite for the /router/quote endpoint
# Test runs tests for exact amount out quotes.
class TestExactAmountOutQuote:
    @pytest.mark.parametrize("coin_obj", construct_token_in_combos(conftest.choose_tokens_liq_range(QUOTE_NUM_TOP_LIQUIDITY_DENOMS), USDC_PRECISION - 1, USDC_PRECISION + 4), ids=id_from_coin)
    def test_usdc_in_high_liq_in(self, environment_url, coin_obj):
        """
        This test case validates quotes between USDC in and NUM_TOP_LIQUIDITY_DENOMS.
        The amounts are constructed to be seeded random values between 10^USDC_PRECISION-1 and 10 ^(USDC_PRECISION + 4)

        This allows us to validate that we can continue to quote at reasonable USDC values for all major token pairs without errors.

        Note: the reason we use Decimal in this test is because floats truncate in some edge cases, leading
        to flakiness.
        """

        denom_in = coin_obj["denom"]
        amount_str = coin_obj["amount_str"]
        amount_out = int(amount_str)

        # Choose the error tolerance based on amount in swapped.
        error_tolerance = Quote.choose_error_tolerance(amount_out)

        # Skip USDC quotes
        if denom_in == USDC:
            return

        denom_in_data = conftest.shared_test_state.chain_denom_to_data_map.get(denom_in)
        denom_in_precision = denom_in_data.get("exponent")
        
        # Compute spot price scaling factor.
        spot_price_scaling_factor = Decimal(10)**6 / Decimal(10)**denom_in_precision

        # Compute expected spot prices
        out_base_in_quote_price = Decimal(denom_in_data.get("price"))
        expected_in_base_out_quote_price = 1 / out_base_in_quote_price
        
        # Compute expected token out
        expected_token_out = int(amount_str) * expected_in_base_out_quote_price

        # Set the token in coin
        token_out_coin = amount_str + USDC

        # Run the quote test
        quote = ExactAmountOutQuote.run_quote_test(environment_url, token_out_coin, denom_in, False, False, EXPECTED_LATENCY_UPPER_BOUND_MS)

        ExactAmountOutQuote.validate_quote_test(quote, amount_str, USDC, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_out, denom_in, error_tolerance)

    # - Constructs combinations between each from 10^6 to 10^9 amount input
    @pytest.mark.parametrize("swap_pair", conftest.create_coins_from_pairs(conftest.create_no_dupl_token_pairs(conftest.choose_tokens_liq_range(num_tokens=10, min_liq=500_000, exponent_filter=USDC_PRECISION)), USDC_PRECISION, USDC_PRECISION + 3), ids=id_from_swap_pair)
    def test_top_liq_combos_default_exponent(self, environment_url, swap_pair):
        token_out_obj = swap_pair['token_in']
        amount_str = token_out_obj['amount_str']
        token_out_denom = token_out_obj['denom']
        token_out_coin = amount_str + token_out_denom
        denom_in = swap_pair['out_denom']
        amount_out = int(amount_str)
        coin = Coin(token_out_denom, amount_str)

        TestExactAmountOutQuote.run_top_liq_combos_default_exponent(environment_url, amount_str, token_out_denom, denom_in)

    @staticmethod
    def run_top_liq_combos_default_exponent(environment_url, amount_str, token_out_denom, denom_in):
        amount_out = int(amount_str)
        token_out_coin = amount_str + token_out_denom
        coin = Coin(token_out_denom, amount_str)

        # All tokens have the same default exponent, resulting in scaling factor of 1.
        spot_price_scaling_factor = 1

        expected_in_base_out_quote_price, expected_token_in, token_out_amount_usdc_value = ExactAmountOutQuote.calculate_expected_base_out_quote_spot_price(denom_in, coin)

        # Chose the error tolerance based on amount in swapped.
        error_tolerance = Quote.choose_error_tolerance(token_out_amount_usdc_value)

        # Run the quote test
        quote = ExactAmountOutQuote.run_quote_test(environment_url, token_out_coin, denom_in, False, False, EXPECTED_LATENCY_UPPER_BOUND_MS)
        # Validate that price impact is present.
        assert quote.price_impact is not None

        # If the token in amount value is less than $HIGH_LIQ_PRICE_IMPACT_CHECK_USD_AMOUNT_IN_THRESHOLD, we expect the price impact to not exceed threshold
        if token_out_amount_usdc_value < HIGH_LIQ_PRICE_IMPACT_CHECK_USD_AMOUNT_IN_THRESHOLD:
             quote.price_impact * -1 < HIGH_LIQ_MAX_PRICE_IMPACT_THRESHOLD, f"Error: price impact is either None or greater than {HIGH_LIQ_MAX_PRICE_IMPACT_THRESHOLD} {quote.price_impact}"

        # Validate quote results
        ExactAmountOutQuote.validate_quote_test(quote, amount_str, token_out_denom, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_in, denom_in, error_tolerance)

    @pytest.mark.parametrize("amount", [str(10**(USDC_PRECISION + 3))])
    def test_transmuter_tokens(self, environment_url, amount):
        """
        This test validates that swapping over a route with a transmuter pool works as expected.

        Swaps amount 10^(USDC_PRECISION + 3) of the first token in the transmuter pool to the second token in the transmuter pool.
        The reason why the amount is large is to avoid flakiness at smaller amounts. Due to no slippage at higher value, we should
        expect to see a transmuter picked up.

        Transmuter pools tend to get imbalanced due to the market dynamics hovering over one of the tokens over time.
        To avoid flakiness, we disable this test if liquidity of one of the tokens in the transmuter pool is less than TRANSMUTER_MIN_TOKEN_LIQ_USD.

        Runs quote validations.

        Asserts that transmuter pool is present in route.
        """
        transmuter_token_data = conftest.shared_test_state.transmuter_token_pairs[0]

        # Skip the transmuter test if any of the tokens in the transmuter pool have less than TRANSMUTER_MIN_TOKEN_LIQ_USD liquidity.
        # See definition of TRANSMUTER_MIN_TOKEN_LIQ_USD for more information.
        skip_imbalanced_pool_test_if_imbalanced(transmuter_token_data)

        transmuter_token_pair = transmuter_token_data[1]

        denom_in = transmuter_token_pair[1]
        denom_out = transmuter_token_pair[0]

        # This is the max error tolerance of 5% that we allow.
        # Arbitrarily hand-picked to avoid flakiness.
        error_tolerance = 0.05

        # Get denom in precision.
        denom_out_precision = conftest.get_denom_exponent(denom_out)

        # Get denom out data to retrieve precision and price 
        denom_in_data = conftest.shared_test_state.chain_denom_to_data_map.get(denom_in)
        denom_in_precision = denom_in_data.get("exponent")
        
        # Compute spot price scaling factor.
        spot_price_scaling_factor = Decimal(10)**denom_out_precision / Decimal(10)**denom_in_precision

        # Compute expected spot prices
        out_base_in_quote_price = Decimal(denom_in_data.get("price"))
        expected_in_base_out_quote_price = 1 / out_base_in_quote_price
        
        # Compute expected token in
        expected_token_in = int(amount) * expected_in_base_out_quote_price

        # Run the quote test
        quote = ExactAmountOutQuote.run_quote_test(environment_url, amount + denom_out, denom_in, False, False, EXPECTED_LATENCY_UPPER_BOUND_MS)

        # Transmuter is expected to be in the route only if the amount out is equal to the amount in
        # in rare cases, CL pools can be picked up instead of transmuter, providing a higher amount out.
        if quote.amount_in == quote.amount_out.amount:
            # Validate transmuter was in route
            assert Quote.is_transmuter_in_single_route(quote.route) is True

        # Validate the quote test
        ExactAmountOutQuote.validate_quote_test(quote, amount, denom_out, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_in, denom_in, error_tolerance)
