
def relative_error(a, b):
    """
    Returns the relative error between two numbers.
    """
    return abs(a - b) / max(abs(a), abs(b))
