def get_last_route_token_out(route):
    token_out_denom = ""
    for pool in route.pools:
        token_out_denom = pool.token_out_denom
    return token_out_denom

def get_last_route_token_in(route):
    token_in_denom = ""
    for pool in route.pools:
        token_in_denom = pool.token_in_denom
    return token_in_denom


def get_last_quote_route_token_in(quote):
    token_in_denom = ""
    for route in quote.route:
        for pool in route.pools:
            token_in_denom = pool.token_in_denom
    return token_in_denom

def get_last_quote_route_token_out(quote):
    token_out_denom = ""
    for route in quote.route:
        for pool in route.pools:
            token_out_denom = pool.token_out_denom
    return token_out_denom
