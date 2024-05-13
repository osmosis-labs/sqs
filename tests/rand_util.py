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
