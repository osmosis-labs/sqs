import requests

COINGECKO_URL = "https://prices.osmosis.zone/api/v3/simple/price"
USD_CURRENCY = "usd"

class CoingeckoService:

    # Given the coingecko id, call the coingecko API endpoint and return its token price
    def get_token_price(self, coingecko_id):
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
        return response_json.get(coingecko_id, {}).get(USD_CURRENCY, None)