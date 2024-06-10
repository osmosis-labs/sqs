import random

def get_random_numbers(seed=42, start_order=6, end_order=11):
    """
    Gets random numbers within each order of magnitude from start_order to end_order, using a specified seed.

    By default it gets random numbers from 10^6 to 10^11.

    The seed is an arbitrary value of 42

    This is helpful for generating test token amounts
    """
    local_random = random.Random(seed)
    return [str(local_random.randint(10**order, 10**(order+1)-1)) for order in range(start_order, end_order + 1)]

def construct_token_in_combos(denoms, start_order, end_order):
    """
    Constructs random token in combinations with random amounts between the given orders
    of magntiude for each denom in the given list.

    For each denom, generates random amounts using the seed of 1 and increasing by 1 for the next denom.

    Returns a list of dictionaries with the following keys
    - denom: str
    - amount_str: str
    """
    test_cases = []

    seed = 1
    
    for denom in denoms:
        random_numbers = get_random_numbers(seed, start_order, end_order)

        for random_number in random_numbers:
            test_cases.append({
                "denom": denom,
                "amount_str": random_number
            })

        seed += 1

    return test_cases
