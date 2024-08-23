import requests

COINGECKO_URL = "https://prices.osmosis.zone/api/v3/simple/price"
USD_CURRENCY = "usd"

class CoingeckoService:
    cache = {}

    # Given the coingecko id, call the coingecko API endpoint and return its token price
    def get_token_price(self, coingecko_id):
        if coingecko_id in self.cache:
            return self.cache[coingecko_id]

        # Set the query parameters
        params = {
            "ids": coingecko_id,
            "vs_currencies": USD_CURRENCY
        }
        # Send the GET request
        response = requests.get(COINGECKO_URL, params=params)
        if response.status_code != 200:
            raise Exception(f"Error fetching price from coingecko: {response.text}")

        response_json = response.json()
        price =  response_json.get(coingecko_id, {}).get(USD_CURRENCY, None)
        
        self.cache[coingecko_id] = price

        return price