from decimal import *

# Coin represents a coin in the /router/quote response
class Coin:
    def __init__(self, denom, amount):
        self.denom = denom
        self.amount = int(amount)

# Pool represents a pool in the /router/quote response
class Pool:
    def __init__(self, id, type, balances, spread_factor, taker_fee, **kwargs):
        self.id = int(id)
        self.type = type
        self.balances = balances
        self.spread_factor = float(spread_factor)
        self.token_in_denom = kwargs.get('token_in_denom', None)
        self.token_out_denom = kwargs.get('token_out_denom', None)
        self.taker_fee = float(taker_fee)
        code_id = kwargs.get('code_id', 0)
        # Only CW pools have code id
        if code_id:
            self.code_id = int(code_id)

# Route represents a route in the /router/quote response
class Route:
    def __init__(self, pools, out_amount, in_amount, **kwargs):
        self.pools = [Pool(**pool) for pool in pools]
        self.out_amount = int(out_amount)
        self.in_amount = int(in_amount)
        # "has-cw-pool" format is unsupported
        self.has_cw_pool = kwargs.get('has-cw-pool', False)

# QuoteExactAmountInResponse represents the response format
# of the /router/quote endpoint for Exact Amount In Quote.
class QuoteExactAmountInResponse:
    def __init__(self, amount_in, amount_out, route, effective_fee, price_impact, in_base_out_quote_spot_price, price_info=None):
        self.amount_in = Coin(**amount_in)
        self.amount_out = int(amount_out)
        self.route = [Route(**r) for r in route]
        self.effective_fee = Decimal(effective_fee)
        self.price_impact = Decimal(price_impact)
        self.in_base_out_quote_spot_price = Decimal(in_base_out_quote_spot_price)
        if price_info:
            self.price_info = price_info

    def get_pool_ids(self):
        pool_ids = []
        for route in self.route:
            for pool in route.pools:
                pool_ids.append(pool.id)
        return pool_ids

    def get_token_out_denoms(self):
        token_out_denoms = []
        for route in self.route:
            for pool in route.pools:
                token_out_denoms.append(pool.token_out_denom)
        return token_out_denoms

# QuoteExactAmountOutResponse represents the response format
# of the /router/quote endpoint for Exact Amount Out Quote.
class QuoteExactAmountOutResponse:
    def __init__(self, amount_in, amount_out, route, effective_fee, price_impact, in_base_out_quote_spot_price):
        self.amount_in = int(amount_in)
        self.amount_out = Coin(**amount_out)
        self.route = [Route(**r) for r in route]
        self.effective_fee = Decimal(effective_fee)
        self.price_impact = Decimal(price_impact)
        self.in_base_out_quote_spot_price = Decimal(in_base_out_quote_spot_price)

    def get_pool_ids(self):
        pool_ids = []
        for route in self.route:
            for pool in route.pools:
                pool_ids.append(pool.id)
        return pool_ids

    def get_token_in_denoms(self):
        token_in_denoms = []
        for route in self.route:
            for pool in route.pools:
                token_in_denoms.append(pool.token_in_denom)
        return token_in_denoms
