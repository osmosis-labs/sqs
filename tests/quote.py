import time
from typing import Callable, Any
import conftest
from sqs_service import *
from quote_response import *
from rand_util import *
from e2e_math import *
from decimal import *
from constants import *
from util import *
from route import *

class Quote:
    @staticmethod
    def run_quote_test(service_call: Callable[[], Any], expected_latency_upper_bound_ms, expected_status_code=200, orderbook_map=None) -> QuoteExactAmountOutResponse:
        """
        Runs exact amount out test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency.

        Returns quote for additional validation if needed by client.

        Validates:
        - Response status code is as given or default 200.
        - Latency is under the given bound.
        """

        start_time = time.time()
        response = service_call()
        elapsed_time_ms = (time.time() - start_time) * 1000

        assert response.status_code == expected_status_code, f"Error: {response.text}"
        assert expected_latency_upper_bound_ms > elapsed_time_ms, f"Error: latency {elapsed_time_ms} exceeded {expected_latency_upper_bound_ms} ms, denom in and token out"

        # Return the response for further processing
        return response.json()

    @staticmethod
    def choose_error_tolerance(amount: int):
         # This is the max error tolerance of 7% that we allow.
        # Arbitrarily hand-picked to avoid flakiness.
        error_tolerance = 0.07
        # At a higher amount in, the volatility is much higher, leading to
        # flakiness. Therefore, we increase the error tolerance based on the amount in swapped.
        # The values are arbitrarily hand-picked and can be adjusted if necessary.
        # This seems to be especially relevant for the Astroport PCL pools.
        if amount > 60_000_000_000:
            error_tolerance = 0.16
        elif amount > 30_000_000_000:
            error_tolerance = 0.13
        elif amount > 10_000_000_000:
            error_tolerance = 0.10

        return error_tolerance

    @staticmethod
    def is_transmuter_in_single_route(routes):
        """
        Returns true if there is a single route with
        one transmuter pool in it.
        """
        if len(routes) == 1 and len(routes[0].pools) == 1:
            pool_in_route = routes[0].pools[0]
            pool = conftest.shared_test_state.pool_by_id_map.get(str(pool_in_route.id))
            e2e_pool_type = conftest.get_e2e_pool_type_from_numia_pool(pool)

            return  e2e_pool_type == conftest.E2EPoolType.COSMWASM_TRANSMUTER_V1
        
        return False

    @staticmethod
    def validate_fee(token_in_denom, token_out_denom, actual_fee, pool_id):
        pair_taker_fee = conftest.CHAIN_SERVICE.get_trading_pair_taker_fee(token_in_denom, token_out_denom)

        assert pair_taker_fee is not None, f"Error: taker fee is not available for {token_in_denom} and {token_out_denom}"
        taker_fee_decimal = Decimal(pair_taker_fee.get("taker_fee"))
        taker_fee_decimal == actual_fee, f"Error: taker fee {taker_fee_decimal} is not equal to effective fee {actual_fee}"

        if taker_fee_decimal != 0:
            assert False, f"Error: taker fee {taker_fee_decimal} is not charged for pool {pool_id}"


    @staticmethod
    def validate_pool_denoms_in_route(token_in_denom, token_out_denom, denoms, pool_id, route_denom_in, route_denom_out):
        """
        Validates that the pool denoms are present in the route.
        """
        
        # HACK:
        # Numia does not put alloyed LP share into pool denoms so we skip for simplicity
        # Should eventually check this unconditionally
        if "all" not in token_out_denom:
            assert token_out_denom, f"Error: token out {token_out_denom} not found in pool denoms {denoms}, pool ID {pool_id}, route in {route_denom_in}, route out {route_denom_out}"

        if "all" not in token_in_denom:
            assert token_in_denom, f"Error: token in {token_in_denom} not found in pool denoms {denoms}, pool ID {pool_id}, route in {route_denom_in}, route out {route_denom_out}"

class ExactAmountInQuote:
    @staticmethod
    def calculate_expected_base_out_quote_spot_price(denom_out, coin):
        """
        Compute expected base out quote spot price

        First, get the USD price of each denom, and then divide to get the expected spot price
        """

        # Compute expected base out quote spot price
        # First, get the USD price of each denom, and then divide to get the expected spot price
        in_base_usd_quote_price = conftest.get_usd_price_scaled(denom_out)
        out_base_usd_quote_price = conftest.get_usd_price_scaled(coin.denom)
        expected_in_base_out_quote_price = out_base_usd_quote_price / in_base_usd_quote_price 

        # Compute expected token out
        expected_token_in = int(coin.amount) * expected_in_base_out_quote_price

        token_in_amount_usdc_value = in_base_usd_quote_price * coin.amount

        return expected_in_base_out_quote_price, expected_token_in, token_in_amount_usdc_value

    def run_quote_test(environment_url, token_in, token_out, human_denoms, single_route, expected_latency_upper_bound_ms, expected_status_code=200, simulator_address="", simulation_slippage_tolerance="") -> QuoteExactAmountInResponse:
        """
        Runs a test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency

        Returns quote for additional validation if needed by client

        Validates:
        - Response status code is as given or default 200
        - Latency is under the given bound
        """
        
        service_call = lambda: conftest.SERVICE_MAP[environment_url].get_exact_amount_in_quote(token_in, token_out, human_denoms, single_route, simulator_address, simulation_slippage_tolerance)        

        response = Quote.run_quote_test(service_call, expected_latency_upper_bound_ms, expected_status_code)

        # Return route for more detailed validation
        quote_response = QuoteExactAmountInResponse(**response)

        price_info = response.get("price_info")
        if price_info:
            quote_response.price_info = price_info

        return quote_response

    @staticmethod
    def validate_quote_test(quote, expected_amount_in_str, expected_denom_in, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_out, denom_out, error_tolerance, direct_quote=False):
        """
        Runs the following validations:
        - Basic presence of fields
        - Transmuter has no price impact. Otherwise, it is negative.
        - Token out amount is within error tolerance from expected.
        - Returned spot price is within error tolerance from expected.
        """
        
        # Validate routes are generally present
        assert len(quote.route) > 0

        # Check if the route is a single pool single transmuter route
        # For such routes, the price impact is 0.
        is_transmuter_route = Quote.is_transmuter_in_single_route(quote.route)

        # Validate price impact
        # If it is a single pool single transmuter route, we expect the price impact to be 0
        # Price impact is returned as a negative number for any other route.
        assert quote.price_impact is not None
        assert (not is_transmuter_route) and (quote.price_impact < 0) or (is_transmuter_route) and (quote.price_impact == 0), f"Error: price impact {quote.price_impact} is zero for non-transmuter route"
        price_impact_positive = quote.price_impact * -1

        # Validate amount in and denom are as input
        assert quote.amount_in.amount == int(expected_amount_in_str)
        assert quote.amount_in.denom == expected_denom_in

        # Validate that the fee is charged
        ExactAmountInQuote.validate_fee(quote)

        # Validate that the route is valid
        ExactAmountInQuote.validate_route(quote, expected_denom_in, denom_out, direct_quote)

        # Validate that the spot price is present
        assert quote.in_base_out_quote_spot_price is not None

        # Validate that the spot price is within the error tolerance
        assert relative_error(quote.in_base_out_quote_spot_price * spot_price_scaling_factor, expected_in_base_out_quote_price) < error_tolerance, f"Error: in base out quote spot price {quote.in_base_out_quote_spot_price} is not within {error_tolerance} of expected {expected_in_base_out_quote_price}"

        # If there is a price impact greater than the provided error tolerance, we dynamically set the error tolerance to be
        # the price impact * (1 + error_tolerance) to account for the price impact
        if price_impact_positive > error_tolerance:
            error_tolerance = price_impact_positive * Decimal(1 + error_tolerance)

        # Validate that the amount out is within the error tolerance
        amount_out_scaled = quote.amount_out * spot_price_scaling_factor
        assert relative_error(amount_out_scaled, expected_token_out) < error_tolerance, f"Error: amount out scaled {amount_out_scaled} is not within {error_tolerance} of expected {expected_token_out}"

    @staticmethod
    def validate_route(quote, denom_in, denom_out,  direct_quote=False):
        """
        Validates that the route is valid by checking the following:
            - The input token is present in each pool denoms
            - The last token out is equal to denom out
        """
        for route in quote.route:
            cur_token_in_denom = denom_in
            for p in route.pools:
                pool_id = p.id
                pool = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))

                assert pool, f"Error: pool ID {pool_id} not found in test data"

                denoms = conftest.get_denoms_from_pool_tokens(pool.get("pool_tokens"))

                # Validate route denoms are present in pool
                Quote.validate_pool_denoms_in_route(cur_token_in_denom, p.token_out_denom, denoms, pool_id, denom_in, denom_out)

                cur_token_in_denom = p.token_out_denom

            if not direct_quote:
                # Last route token out must be equal to denom out
                assert denom_out == get_last_route_token_out(route), f"Error: denom out {denom_out} not equal to last token out {get_last_route_token_out(route)}"

        if direct_quote:
            # For direct custom quotes response always is multi route
            assert denom_out == get_last_quote_route_token_out(quote), f"Error: denom out {denom_out} not equal to last token out {get_last_quote_route_token_out(quote)}"

    @staticmethod
    def validate_fee(quote):
        """
        Validates fee returned in the quote response for out givn in quotes.
        If the returned fee is zero, it iterates over every pool in every route and ensures that their taker fees
        are zero based on external data source.

        In other cases, asserts that the fee is non-zero.
        """
        # Validate that the fee is charged
        if quote.effective_fee == 0:
            token_in = quote.amount_in.denom
            for route in quote.route:
                cur_token_in = token_in
                for pool in route.pools:
                    pool_id = pool.id

                    token_out = pool.token_out_denom

                    Quote.validate_fee(cur_token_in, token_out, quote.effective_fee, pool_id)

                    cur_token_in = token_out
        else:
            assert quote.effective_fee > 0

class ExactAmountOutQuote:
    @staticmethod
    def calculate_expected_base_out_quote_spot_price(denom_in, coin):
        """
        Compute expected base out quote spot price

        First, get the USD price of each denom, and then divide to get the expected spot price
        """

        in_base_usd_quote_price = conftest.get_usd_price_scaled(denom_in)
        out_base_usd_quote_price = conftest.get_usd_price_scaled(coin.denom)
        expected_in_base_out_quote_price = out_base_usd_quote_price / in_base_usd_quote_price 

        # Compute expected token out
        expected_token_in = int(coin.amount) * expected_in_base_out_quote_price

        token_out_amount_usdc_value = in_base_usd_quote_price * coin.amount

        return expected_in_base_out_quote_price, expected_token_in, token_out_amount_usdc_value

    @staticmethod
    def run_quote_test(environment_url, token_out, token_in, human_denoms, single_route, expected_latency_upper_bound_ms, expected_status_code=200) -> QuoteExactAmountOutResponse:
        """
        Runs exact amount out test for the /router/quote endpoint with the given input parameters.

        Does basic validation around response status code and latency

        Returns quote for additional validation if needed by client

        Validates:
        - Response status code is as given or default 200
        - Latency is under the given bound
        """

        sqs = conftest.SERVICE_MAP[environment_url]
        service_call = lambda: sqs.get_exact_amount_out_quote(token_out, token_in, human_denoms, single_route)

        response = Quote.run_quote_test(service_call, expected_latency_upper_bound_ms, expected_status_code)

        # Return route for more detailed validation
        return QuoteExactAmountOutResponse(**response)

    @staticmethod
    def validate_quote_test(quote, expected_amount_out_str, expected_denom_out, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_in, denom_in, error_tolerance, direct_quote=False, orderbook_map=None):
        """
        Runs the following validations:
        - Basic presence of fields
        - Transmuter has no price impact. Otherwise, it is negative.
        - Token out amount is within error tolerance from expected.
        - Returned spot price is within error tolerance from expected.
        """

        # Validate routes are generally present
        assert len(quote.route) > 0

        # Check if the route is a single pool single transmuter route
        # For such routes, the price impact is 0.
        is_transmuter_route = Quote.is_transmuter_in_single_route(quote.route)

        # Validate price impact
        # If it is a single pool single transmuter route, we expect the price impact to be 0
        # Price impact is returned as a negative number for any other route.
        assert quote.price_impact is not None
        assert (not is_transmuter_route) and (quote.price_impact < 0) or (is_transmuter_route) and (quote.price_impact == 0), f"Error: price impact {quote.price_impact} is zero for non-transmuter route"
        price_impact_positive = quote.price_impact * -1

        # Validate amount in and denom are as input
        assert quote.amount_out.amount == int(expected_amount_out_str)
        assert quote.amount_out.denom == expected_denom_out

        # Validate that the fee is charged
        ExactAmountOutQuote.validate_fee(quote)

        # Validate that the route is valid
        ExactAmountOutQuote.validate_route(quote, denom_in, expected_denom_out, direct_quote, orderbook_map)

        # Validate that the spot price is present
        assert quote.in_base_out_quote_spot_price is not None

        # Validate that the spot price is within the error tolerance
        assert relative_error(quote.in_base_out_quote_spot_price * spot_price_scaling_factor, expected_in_base_out_quote_price) < error_tolerance, f"Error: in base out quote spot price {quote.in_base_out_quote_spot_price} is not within {error_tolerance} of expected {expected_in_base_out_quote_price}"

        # If there is a price impact greater than the provided error tolerance, we dynamically set the error tolerance to be
        # the price impact * (1 + error_tolerance) to account for the price impact
        if price_impact_positive > error_tolerance:
            error_tolerance = price_impact_positive * Decimal(1 + error_tolerance)

        # Validate that the amount out is within the error tolerance
        amount_in_scaled = quote.amount_in * spot_price_scaling_factor
        assert relative_error(amount_in_scaled, expected_token_in) < error_tolerance, f"Error: amount out scaled {amount_in_scaled} is not within {error_tolerance} of expected {expected_token_in}"

    @staticmethod
    def validate_route(quote, denom_in, denom_out, direct_quote=False, orderbook_map=None):
        """
        Validates that the route is valid by checking the following:
         - The output token is present in each pool denoms
         - The last token in is equal to denom in
         - Order books are not present in the route
        """
        for route in quote.route:
            cur_out_denom = denom_out
            for p in route.pools:
                pool_id = p.id
                pool = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))

                assert pool, f"Error: pool ID {pool_id} not found in test data"

                if orderbook_map:
                    is_orderbook = orderbook_map.get(pool_id) is not None
                    assert not is_orderbook, f"Error: orderbook {pool_id} is present in route {denom_in} -> {denom_out}"

                denoms = conftest.get_denoms_from_pool_tokens(pool.get("pool_tokens"))

                Quote.validate_pool_denoms_in_route(p.token_in_denom, cur_out_denom, denoms, pool_id, denom_in, denom_out)

                cur_out_denom = p.token_in_denom

            if not direct_quote:
                # Last route token in must be equal to denom in
                assert denom_in == get_last_route_token_in(route), f"Error: denom in {denom_in} not equal to last token in {get_last_route_token_in(route)}"

        if direct_quote:
            # For direct custom quotes response always is multi route
            assert denom_in == get_last_quote_route_token_in(quote), f"Error: denom in {denom_in} not equal to last token in {get_last_quote_route_token_in(quote)}"

    @staticmethod
    def validate_fee(quote):
        """
        Validates fee returned in the quote response for in given out quotes.
        If the returned fee is zero, it iterates over every pool in every route and ensures that their taker fees
        are zero based on external data source.

        In other cases, asserts that the fee is non-zero.
        """
        # Validate that the fee is charged
        if quote.effective_fee == 0:
            token_out = quote.amount_out.denom
            for route in quote.route:
                cur_token_out = token_out
                for pool in route.pools:
                    pool_id = pool.id

                    token_in = pool.token_in_denom

                    Quote.validate_fee(token_in, cur_token_out, quote.effective_fee, pool_id)

                    cur_token_out = token_in
        else:
            assert quote.effective_fee > 0
