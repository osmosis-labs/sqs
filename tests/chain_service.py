import requests

PUBLIC_CHAIN_ENDPOINT = "https://lcd.osmosis.zone"

TAKER_FEE_URL = "/osmosis/poolmanager/v1beta1/trading_pair_takerfee"

class ChainService:
    def __init__(self):
        self.url = PUBLIC_CHAIN_ENDPOINT
        self.taker_fee_map = {}

    def get_trading_pair_taker_fee(self, denom0, denom1):
        """
        Fetches the trading pair taker fee by denoms and caches them

        Raises error if non-200 is returned from the endpoint.
        """

        sortedDenoms = sorted([denom0, denom1])
        
        taker_fee = self.taker_fee_map.get((denom0, denom1), None)
        if taker_fee is not None:
            return taker_fee

        response = requests.get(self.url + TAKER_FEE_URL, params={"denom_0": sortedDenoms[0], "denom_1": sortedDenoms[1]})

        if response.status_code != 200:
            raise Exception(f"Error fetching config: {response.text}")

        taker_fee = response.json()

        self.taker_fee_map[(denom0, denom1)] = taker_fee

        return taker_fee
