import time
import pytest
import itertools

import conftest
from sqs_service import *
from quote import *
from quote_response import *
from rand_util import *
from e2e_math import *
from decimal import *
from constants import *
from chain_service import *
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


def orderbook_token_pairs():
    """
    Returns a list of tuples of the form (pool_id, (token_in, token_out))
    """
    return [
        (pool_id, pair) \
            for [pool_id, denoms] in conftest.shared_test_state.orderbook_token_pair \
                for pair in itertools.combinations(denoms, 2)
    ]
    

# Test suite for the /router/quote endpoint
# Test runs tests for exact amount in quotes.
class TestExactAmountInQuote:
    @pytest.mark.parametrize("coin_obj", construct_token_in_combos(conftest.choose_tokens_liq_range(QUOTE_NUM_TOP_LIQUIDITY_DENOMS), USDC_PRECISION - 1, USDC_PRECISION + 4), ids=id_from_coin)
    def test_usdc_in_high_liq_out(self, environment_url, coin_obj):
        """
        This test case validates quotes betwen USDC in and NUM_TOP_LIQUIDITY_DENOMS.
        The amounts are constructed to be seeded random values between 10^USDC_PRECISION-1 and 10 ^(USDC_PRECISION + 4)

        This allows us to validate that we can continue to quote at reasonable USDC values for all majore token pairs without errors.

        Note: the reason we use Decimal in this test is because floats truncate in some edge cases, leading
        to flakiness.
        """

        denom_out = coin_obj["denom"]
        amount_str = coin_obj["amount_str"]
        amount_in = int(amount_str)

        # Choosse the error tolerance based on amount in swapped.
        error_tolerance = Quote.choose_error_tolerance(amount_in)

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
        expected_token_out = int(amount_str) * expected_in_base_out_quote_price

        # Set the token in coin
        token_in_coin = amount_str + USDC
        # Run the quote test
        quote =  ExactAmountInQuote.run_quote_test(environment_url, token_in_coin, denom_out, False, False, EXPECTED_LATENCY_UPPER_BOUND_MS)

        ExactAmountInQuote.validate_quote_test(quote, amount_str, USDC, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_out, denom_out, error_tolerance)

    # - Constructs combinations between each from 10^6 to 10^9 amount input
    @pytest.mark.parametrize("swap_pair", conftest.create_coins_from_pairs(conftest.create_no_dupl_token_pairs(conftest.choose_tokens_liq_range(num_tokens=10, min_liq=500_000, exponent_filter=USDC_PRECISION)), USDC_PRECISION, USDC_PRECISION + 3), ids=id_from_swap_pair)
    def test_top_liq_combos_default_exponent(self, environment_url, swap_pair):
        token_in_obj = swap_pair['token_in']
        amount_str = token_in_obj['amount_str']
        token_in_denom = token_in_obj['denom']
        denom_out = swap_pair['out_denom']

        TestExactAmountInQuote.run_top_liq_combos_default_exponent(environment_url, amount_str, token_in_denom, denom_out)

    @staticmethod
    def run_top_liq_combos_default_exponent(environment_url, amount_str, token_in_denom, denom_out):
        amount_in = int(amount_str)
        token_in_coin = amount_str + token_in_denom
        coin = Coin(token_in_denom, amount_str)

        # All tokens have the same default exponent, resulting in scaling factor of 1.
        spot_price_scaling_factor = 1

        expected_in_base_out_quote_price, expected_token_out, token_in_amount_usdc_value = ExactAmountOutQuote.calculate_expected_base_out_quote_spot_price(denom_out, coin)

        # Choosse the error tolerance based on amount in swapped.
        error_tolerance = Quote.choose_error_tolerance(token_in_amount_usdc_value)

        # Run the quote test
        quote = ExactAmountInQuote.run_quote_test(environment_url, token_in_coin, denom_out, False, False, EXPECTED_LATENCY_UPPER_BOUND_MS)
        # Validate that price impact is present.
        assert quote.price_impact is not None

        # If the token in amount value is less than $HIGH_LIQ_PRICE_IMPACT_CHECK_USD_AMOUNT_IN_THRESHOLD, we expect the price impact to not exceed threshold
        if token_in_amount_usdc_value < HIGH_LIQ_PRICE_IMPACT_CHECK_USD_AMOUNT_IN_THRESHOLD:
                quote.price_impact * -1 < HIGH_LIQ_MAX_PRICE_IMPACT_THRESHOLD, f"Error: price impact is either None or greater than {HIGH_LIQ_MAX_PRICE_IMPACT_THRESHOLD} {quote.price_impact}"

        # Validate quote results
        ExactAmountInQuote.validate_quote_test(quote, amount_str, token_in_denom, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_out, denom_out, error_tolerance)

    def test_custom_direct_quote_single_hop(self, environment_url,):
        """
        This test validates that direct quotes work as expected for a single hop.
        """
        sqs_service = conftest.SERVICE_MAP[environment_url]

        amount = 1000000
        denom_in = constants.USDC
        denom_out = constants.UOSMO

        response = sqs_service.get_exact_amount_in_custom_direct_quote(str(amount) + denom_in, denom_out, constants.UOSMO_USDC_POOL_ID)
        assert response.status_code == 200, f"Error: {response.text}"

        res = response.json()

        quote = QuoteExactAmountInResponse(**res)

        # Basic sanity checks that the results are present
        quote.amount_in.amount == amount
        quote.amount_in.denom == denom_in
        quote.amount_out > 0

    def test_custom_direct_quote_multi_hop(self, environment_url,):
        """
        This test validates that custom direct quotes work as expected for a multi hop.

        It attempts to swap over a hardcoded route with multiple hops.
        """
        sqs_service = conftest.SERVICE_MAP[environment_url]

        amount = 1000000
        denom_in = constants.USDC
        wbtc = "factory/osmo1z0qrq605sjgcqpylfl4aa6s90x738j7m58wyatt0tdzflg2ha26q67k743/wbtc"
        allBtc = "factory/osmo1z6r6qdknhgsc0zeracktgpcxf43j6sekq07nw8sxduc9lg0qjjlqfu25e3/alloyed/allBTC"

        response = sqs_service.get_exact_amount_in_custom_direct_quote(str(amount) + denom_in, f"{wbtc},{allBtc}", "1436,1868")
        assert response.status_code == 200, f"Error: {response.text}"

        res = response.json()

        quote = QuoteExactAmountInResponse(**res)

        # Basic sanity checks that the results are present
        quote.amount_in.amount == amount
        quote.amount_in.denom == denom_in
        quote.amount_out > 0


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

        denom_in = transmuter_token_pair[0]
        denom_out = transmuter_token_pair[1]

        # This is the max error tolerance of 5% that we allow.
        # Arbitrarily hand-picked to avoid flakiness.
        error_tolerance = 0.05

        # Get denom in precision.
        denom_in_precision = conftest.get_denom_exponent(denom_in)

        # Get denom out data to retrieve precision and price 
        denom_out_data = conftest.shared_test_state.chain_denom_to_data_map.get(denom_out)
        denom_out_precision = denom_out_data.get("exponent")
        
        # Compute spot price scaling factor.
        spot_price_scaling_factor = Decimal(10)**denom_in_precision / Decimal(10)**denom_out_precision

        # Compute expected spot prices
        out_base_in_quote_price = Decimal(denom_out_data.get("price"))
        expected_in_base_out_quote_price = 1 / out_base_in_quote_price
        
        # Compute expected token out
        expected_token_out = int(amount) * expected_in_base_out_quote_price

        # Run the quote test
        quote = ExactAmountInQuote.run_quote_test(environment_url, amount + denom_in, denom_out, False, False, EXPECTED_LATENCY_UPPER_BOUND_MS)

        # Transmuter is expected to be in the route only if the amount out is equal to the amount in
        # in rare cases, CL pools can be picked up instead of transmuter, providing a higher amount out.
        if quote.amount_out == quote.amount_in.amount:
            # Validate transmuter was in route
            assert Quote.is_transmuter_in_single_route(quote.route) is True

        # Validate the quote test
        ExactAmountInQuote.validate_quote_test(quote, amount, denom_in, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_out, denom_out, error_tolerance)

    @pytest.mark.parametrize("amount", [1000])
    @pytest.mark.parametrize("token_pair", orderbook_token_pairs())
    def test_orderbook(self, environment_url, amount, token_pair):
        """
        This test aims to validate the orderbook functionality by performing a direct quote request.
        
        The amount out from direct quote is compared to the amount out calculated by amount in after fee * spot price
        to ensure that the orderbook quote is working as expected.

        Small amount in is used since spot price is calculated based on the tick price at the top of the orderbook.
        If the amount in is large, the actual amount out will be different from the amount out calculated by amount in after fee * spot price
        due to the moving ticks and potential low liquidity on one side of the orderbook.
        """
        (pool_id, (denom_in, denom_out)) = token_pair

        sqs_service = conftest.SERVICE_MAP[environment_url]
        start_time = time.time()
        response = sqs_service.get_exact_amount_in_custom_direct_quote(str(amount) + denom_in, denom_out, pool_id)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == 200, f"Error: {response.text}"
        assert elapsed_time_ms < EXPECTED_LATENCY_UPPER_BOUND_MS, \
             f"Error: latency {elapsed_time_ms} exceeded {EXPECTED_LATENCY_UPPER_BOUND_MS} ms, token in {denom_in} and token out {denom_out}" 

        res = response.json()
        
        in_base_out_quote_spot_price = Decimal(res["in_base_out_quote_spot_price"])
        amount_in = int(res["amount_in"]["amount"])
        amount_out = int(res["amount_out"])
        effective_fee = Decimal(res["effective_fee"])
        amount_in_after_fee = int(amount_in * (1 - effective_fee))

        denom_in_data = conftest.shared_test_state.chain_denom_to_data_map.get(denom_in)
        denom_out_data = conftest.shared_test_state.chain_denom_to_data_map.get(denom_out)

        denom_in_precision = denom_in_data.get("exponent")
        denom_out_precision = denom_out_data.get("exponent")

        # Compute expected spot prices
        in_price = Decimal(denom_in_data.get("price")) 
        out_price = Decimal(denom_out_data.get("price"))
        expected_in_base_out_quote_price = (in_price * Decimal(10)**denom_out_precision) / (out_price * Decimal(10)**denom_in_precision)

        # Compute expected token out
        expected_amount_out = amount_in_after_fee * expected_in_base_out_quote_price
        error_tolerance = Quote.choose_error_tolerance(amount_in_after_fee)

        numia_sqs_price_diff = relative_error(expected_in_base_out_quote_price, in_base_out_quote_spot_price)
        assert numia_sqs_price_diff < error_tolerance, \
            f"Error: difference between numia spot price and sqs spot price is {numia_sqs_price_diff} which is greater than {error_tolerance}"

        amount_out_diff = relative_error(expected_amount_out, amount_out)
        assert amount_out_diff < error_tolerance, \
            f"Error: difference between calculated and actual amount out is {amount_out_diff} which is greater than {error_tolerance}"


    def test_simulation_slippage_tolerance(self, environment_url):
        """
        This test validates that the simulation slippage tolerance is working as expected.
        """
        token_in_coin = "1000000uosmo"
        denom_out = "uion"

        expected_status_code = 200

        # Fillbot address and slippage tolerance
        # We choose fillbot address because we expect it to have at least one OSMO.
        fillbot_address = "osmo10s3vlv40h64qs2p98yal9w0tpm4r30uyg6ceux"
        # Note: relaxed
        simulation_slippage_tolerance = 0.8

                # Run the quote test
        quote =  ExactAmountInQuote.run_quote_test(environment_url, token_in_coin, denom_out, False, True, EXPECTED_LATENCY_UPPER_BOUND_MS, expected_status_code, fillbot_address,  simulation_slippage_tolerance)

        # Validate that the price info is set to something
        assert quote is not None
        assert quote.price_info is not None
