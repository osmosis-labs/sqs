import conftest

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
