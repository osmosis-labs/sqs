import requests
import os

COINGECKO_URL = "https://prices.osmosis.zone/api/v3/simple/price"
USD_CURRENCY = "usd"

class CoingeckoService:

    # Caching token price is still acceptable for test purposes 
    # since the token price is not expected to change a lot during test executions
    # key => coingecko_id, value => token price
    cache = {}

    def __init__(self, coingecko_api_key):
        self.coingecko_api_key = coingecko_api_key

    # Check if the service is available
    def isServiceAvailable(self):
        return self.coingecko_api_key is not None

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
        # Set the auth header if the API key is provided
        # If not provided, the request will be sent without the auth header 
        # but expect a lower rate limit and getting HTTP 429 error
        if self.coingecko_api_key is not None:
            headers = {
                "X-API-KEY": self.coingecko_api_key
            }
            response = requests.get(COINGECKO_URL, params=params, headers=headers)
        else:
            response = requests.get(COINGECKO_URL, params=params)

        if response.status_code == 429:
            raise Exception(f"Too many requests to {COINGECKO_URL}: {response.text}")
        if response.status_code != 200:
            raise Exception(f"Error fetching price from coingecko: {response.text}")

        response_json = response.json()
        price =  response_json.get(coingecko_id, {}).get(USD_CURRENCY, None)
        
        self.cache[coingecko_id] = price

        return price