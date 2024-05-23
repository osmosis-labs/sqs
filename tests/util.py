
def id_from_coin(coin_obj):
    """
    Returns a string id from a coin object. This is useful for generating meaningful test
    IDs for parametrized tests.
    """
    if coin_obj is None:
        return "None"
    return f"{coin_obj['amount_str'] + coin_obj['denom']}"

def id_from_swap_pair(swap_pair):
    """
    Returns a string id from a swap pair object that contains token_in and out_denom. This is useful for generating meaningful test
    IDs for parametrized tests.
    """
    if swap_pair is None:
        return "None"

    token_in_obj = swap_pair['token_in']
    token_in_str = token_in_obj['amount_str'] + token_in_obj['denom']

    return f"{token_in_str + '-' + swap_pair['out_denom']}"