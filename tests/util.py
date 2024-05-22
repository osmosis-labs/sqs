
def id_from_coin(coin_obj):
    """
    Returns a string id from a coin object. This is useful for generating meaningful test
    IDs for parametrized tests.
    """

    # This function creates a custom ID for each test case
    if coin_obj is None:
        return "None"
    return f"{coin_obj['amount_str'] + coin_obj['denom']}"