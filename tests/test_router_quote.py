import setup
import time
import pytest

from sqs_service import *
import constants
from conftest import SERVICE_MAP
from quote_response import *
from rand_util import *
from e2e_math import *
from decimal import *

SQS_STAGE = "https://sqs.stage.osmosis.zone"

ROUTES_URL = "/router/quote"

NUM_TOP_LIQUIDITY_DENOMS = 20

# Arbitrary choice based on performance at the time of test writing
expected_latency_upper_bound_ms = 1000

def idfn(coin_obj):
    # This function creates a custom ID for each test case
    if coin_obj is None:
        return "None"
    return f"{coin_obj['amount_str'] + coin_obj['denom']}"

# Test suite for the /router/quote endpoint
class TestQuote:

    @pytest.mark.parametrize("coin_obj", construct_token_in_combos(setup.choose_tokens_liq_range(NUM_TOP_LIQUIDITY_DENOMS), constants.USDC_PRECISION - 1, constants.USDC_PRECISION + 4), ids=idfn)
    def test_usdc_in_high_liq_out(self, environment_url, coin_obj):
        """
        This test case validates quotes betwen USDC in and NUM_TOP_LIQUIDITY_DENOMS.
        The amounts are constructed to be seeded random values between 10^constants.USDC_PRECISION-1 and 10 ^(constants.USDC_PRECISION + 4)

        This allows us to validate that we can continue to quote at reasonable USDC values for all majore token pairs without errors.

        Note: the reason we use Decimal in this test is because floats truncate in some edge cases, leading
        to flakiness.
        """

        # This is the max error tolerance of 5% that we allow.
        error_tolerance = 0.05

        denom_out = coin_obj["denom"]
        amount_str = coin_obj["amount_str"]

        # Skip USDC quotes
        if denom_out == constants.USDC:
            return

        token_in_coin = amount_str + constants.USDC

        denom_out_data = setup.chain_denom_to_data_map.get(denom_out)
        denom_out_precision = denom_out_data.get("exponent")
        
        spot_price_scaling_factor = Decimal(10)**6 / Decimal(10)**denom_out_precision

        out_base_in_quote_price = Decimal(denom_out_data.get("price"))

        quote = self.run_quote_test(environment_url, token_in_coin, denom_out, expected_latency_upper_bound_ms)


        # Validate routes are generally present
        assert len(quote.route) > 0

        is_transmuter_route = False
        if len(quote.route) == 1 and len(quote.route[0].pools) == 1:
            pool_in_route = quote.route[0].pools[0]

            pool = setup.pool_by_id_map.get(pool_in_route.id)

            e2e_type = setup.get_e2e_pool_type_from_numia_pool(pool)

            if e2e_type == setup.E2EPoolType.COSMWASM_TRANSMUTER_V1:
                is_transmuter_route = True

        assert quote.price_impact is not None

        # If it is a transmuter route, we expect the price impact to be 0
        # Price impact is returned as a negative number
        assert not is_transmuter_route and quote.price_impact < 0
        price_impact_positive = quote.price_impact * -1


        # Validate amount in and denom are as input
        assert quote.amount_in.amount == int(amount_str)
        assert quote.amount_in.denom == constants.USDC

        # Validate that the fee is charged
        assert quote.effective_fee > 0

        # Validate that the spot price is present
        assert quote.in_base_out_quote_spot_price is not None


        in_base_out_quote_price = 1 / out_base_in_quote_price
        expected_token_out = int(amount_str) / out_base_in_quote_price

        assert relative_error(quote.in_base_out_quote_spot_price * spot_price_scaling_factor, in_base_out_quote_price) < error_tolerance, f"Error: in base out quote spot price {quote.in_base_out_quote_spot_price} is not within {error_tolerance} of expected {in_base_out_quote_price}"

        # If there is a price impact greater than the provided error tolerance, we dynamically set the error tolerance to be
        # the price impact * (1 + error_tolerance) to account for the price impact
        if price_impact_positive > error_tolerance:
            error_tolerance = price_impact_positive * Decimal(1 + error_tolerance)

        assert relative_error(quote.amount_out * spot_price_scaling_factor, expected_token_out) < error_tolerance, f"Error: amount out {quote.amount_out} is not within {error_tolerance} of expected {expected_token_out}"

    def run_quote_test(self, environment_url, token_in, token_out, expected_latency_upper_bound_ms, expected_status_code=200) -> QuoteResponse:
        """
        Runs a test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency

        Returns quote for additional validation if needed by client

        Validates:
        - Response status code is as given or default 200
        - Latency is under the given bound
        """
        
        sqs_service = SERVICE_MAP[environment_url]

        start_time = time.time()
        response = sqs_service.get_quote(token_in, token_out)
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == expected_status_code, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, token in {token_in} and token out {token_out}" 

        response_json = response.json()

        # Return route for more detailed validation
        return QuoteResponse(**response_json)
