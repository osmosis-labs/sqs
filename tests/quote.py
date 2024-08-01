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


class Quote:
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
    def validate_fee(quote):
        """
        Validates fee returned in the quote response.
        If the returned fee is zero, it iterates over every pool in every route and ensures that their fee
        is zero based on external data source.

        In other cases, asserts that the fee is non-zero.
        """
        # Validate that the fee is charged
        if quote.effective_fee == 0:
            for route in quote.route:
                for pool in route.pools:
                    pool_id = pool.id
                    pool_data = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))
                    swap_fee = pool_data.get("swap_fees")

                    if swap_fee != 0:
                        assert False, f"Error: swap fee {swap_fee} is not charged for pool {pool_id}"
        else:
            assert quote.effective_fee > 0


class ExactAmountOutQuote:
    @staticmethod
    def calculate_amount_transmuter(tokenOut: Coin, denom_in):
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

        return spot_price_scaling_factor, expected_token_in, error_tolerance

    def calculate_amount(tokenOut: Coin, denom_in):
        # All tokens have the same default exponent, resulting in scaling factor of 1.
        spot_price_scaling_factor = 1

        token_out_denom = tokenOut.denom
        amount_str = tokenOut.amount
        amount_out = int(amount_str)

        # Compute expected base out quote spot price
        # First, get the USD price of each denom, and then divide to get the expected spot price
        in_base_usd_quote_price = conftest.get_usd_price_scaled(denom_in)
        out_base_usd_quote_price = conftest.get_usd_price_scaled(token_out_denom)
        expected_in_base_out_quote_price = out_base_usd_quote_price / in_base_usd_quote_price 

        # Compute expected token out
        expected_token_in = int(amount_str) * expected_in_base_out_quote_price

        return in_base_usd_quote_price * amount_out

    @staticmethod
    def validate_quote_test(quote, expected_amount_out_str, expected_denom_out, spot_price_scaling_factor, expected_in_base_out_quote_price, expected_token_in, denom_in, error_tolerance):
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
        Quote.validate_fee(quote)

        # Validate that the route is valid
        ExactAmountOutQuote.validate_route(quote, denom_in, expected_denom_out,)

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
        assert relative_error(amount_in_scaled, expected_token_in) < error_tolerance, f"Error: amount out scaled {amount_out_scaled} is not within {error_tolerance} of expected {expected_token_out}"

    @staticmethod
    def validate_route(quote, denom_in, denom_out):
        """
        Validates that the route is valid by checking the following:
         - The output token is present in each pool denoms
         - The last token in is equal to denom in
        """
        for route in quote.route:
            output = denom_out
            for p in route.pools:
                pool_id = p.id
                pool = conftest.shared_test_state.pool_by_id_map.get(str(pool_id))

                assert pool, f"Error: pool ID {pool_id} not found in test data"

                denoms = conftest.get_denoms_from_pool_tokens(pool.get("pool_tokens"))

                # Pool denoms must contain output denom
                assert output in denoms, f"Error: output {output} not found in pool {pool_id} denoms {denoms}"

                # Pool denoms must contain route input denom
                assert p.token_in_denom in denoms, f"Error: pool token_in_denom {p.token_in_denom} not found in pool {pool_id} denoms {denoms}"

                output = p.token_in_denom

            # Last route token in must be equal to denom in
            assert denom_in == get_last_route_token_in(route), f"Error: denom in {denom_in} not equal to last token in {get_last_route_token_in(route)}"
