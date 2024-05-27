import requests

SQS_STAGE = "https://sqs-eu.stage.osmosis.zone"
SQS_PROD = "https://sqs.osmosis.zone"

ROUTER_ROUTES_URL = "/router/routes"
ROUTER_QUOTE_URL = "/router/quote"

TOKENS_METADATA_URL = "/tokens/metadata"

TOKENS_PRICES_URL = "/tokens/prices"

CONFIG_URL = "/config"

ASSET_LIST_URL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/generated/frontend/assetlist.json"

class SQSService:
    def __init__(self, url):
        self.url = url
        self.tokens_metadata = None
        self.config = None
        self.asset_list = None

    def get_config(self):
        """
        Fetches the config from the specified endpoint and returns it.
        Caches it internally to avoid fetching it multiple times.
        
        Raises error if non-200 is returned from the endpoint.
        """
        if self.config:
            return self.config

        response = requests.get(self.url + CONFIG_URL)

        if response.status_code != 200:
            raise Exception(f"Error fetching config: {response.text}")
        
        self.config = response.json()

        return self.config

    def get_candidate_routes(self, denom_in, denom_out, human_denoms="false"):
        # Set the query parameters
        params = {
            "tokenIn": denom_in,
            "tokenOutDenom": denom_out,
            "humanDenoms": human_denoms
        }

        # Send the GET request
        return requests.get(self.url + ROUTER_ROUTES_URL, params=params)

    def get_quote(self, denom_in, denom_out, human_denoms="false", singleRoute="false"):
        """
        Fetches quote from the specified endpoint and returns it.
        
        Raises error if non-200 is returned from the endpoint.
        """

            # Set the query parameters
        params = {
            "tokenIn": denom_in,
            "tokenOutDenom": denom_out,
            "humanDenoms": human_denoms,
            "singleRoute": singleRoute,
        }

        print(params)

        # Send the GET request
        return requests.get(self.url + ROUTER_QUOTE_URL, params=params)

    def get_tokens_metadata(self):
        """
        Fetches tokens metadata from the specified endpoint and returns them.
        Caches them internally to avoid fetching them multiple times.
        
        Raises error if non-200 is returned from the endpoint.
        """
        if self.tokens_metadata:
            return self.tokens_metadata

        response = requests.get(self.url + TOKENS_METADATA_URL)

        if response.status_code != 200:
            raise Exception(f"Error fetching tokens metadata: {response.text}")
        
        self.tokens_metadata = response.json()

        return self.tokens_metadata

    # Given the base and quote denom, call the SQS API endpoint /tokens/prices
    # and return the token price in the response
    def get_tokens_prices(self, denoms, human_denoms="false"):
        # Set the query parameters
        params = {
            "base": ",".join(denoms),
            "humanDenoms": human_denoms
        }
        # Send the GET request
        response = requests.get(self.url + TOKENS_PRICES_URL, params=params)
        if response.status_code != 200:
            raise Exception(f"Error fetching token price: {response.text}")
        
        return response.json()

    # Given the chain denom, fetch the asset list, parse it and return its coingecko id
    # Asset list is cached internally for performance reasons
    def get_coingecko_id(self, denom):
        coin_minimal_denom_key = "coinMinimalDenom"
        coingecko_id_key = "coingeckoId"
        if self.asset_list == None:
            self.asset_list = {}
            response = requests.get(ASSET_LIST_URL)
            if response.status_code != 200:
                raise Exception(f"Error fetching asset list: {response.text}")
            asset_list_json = response.json()
            for asset in asset_list_json.get("assets"):
                self.asset_list[asset[coin_minimal_denom_key]] = asset.get(coingecko_id_key, None)
            
        return self.asset_list.get(denom, None) 
