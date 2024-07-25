import requests

SQS_STAGE = "https://sqs.stage.osmosis.zone"
SQS_PROD = "https://sqs.osmosis.zone"
SQS_LOCAL = "http://localhost:9092"

ROUTER_ROUTES_URL = "/router/routes"
ROUTER_QUOTE_URL = "/router/quote"

TOKENS_METADATA_URL = "/tokens/metadata"
TOKENS_PRICES_URL = "/tokens/prices"

POOLS_URL = "/pools"
CANONICAL_ORDERBOOKS_URL = "/pools/canonical-orderbooks"

CONFIG_URL = "/config"

ASSET_LIST_URL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/generated/frontend/assetlist.json"

class SQSService:
    def __init__(self, url, api_key):
        self.url = url
        self.tokens_metadata = None
        self.config = None
        self.asset_list = None

        headers={}
        if api_key is not None:
            headers["x-api-key"] = api_key

        self.headers = headers

    def get_config(self):
        """
        Fetches the config from the specified endpoint and returns it.
        Caches it internally to avoid fetching it multiple times.

        Raises error if non-200 is returned from the endpoint.
        """
        if self.config:
            return self.config

        response = requests.get(self.url + CONFIG_URL, headers=self.headers)

        if response.status_code != 200:
            raise Exception(f"Error fetching config: {response.text}")

        self.config = response.json()

        return self.config
    
    def get_pool(self, pool_id):
        """
        Fetches the pool from the specified endpoint and returns it.
        Raises error if non-200 is returned from the endpoint.
        """
        response = requests.get(self.url + f"{POOLS_URL}?IDs={pool_id}", headers=self.headers)

        if response.status_code != 200:
            raise Exception(f"Error fetching pool: {response.text}")

        return response.json()

    def get_candidate_routes(self, denom_in, denom_out, human_denoms="false"):
        # Set the query parameters
        params = {
            "tokenIn": denom_in,
            "tokenOutDenom": denom_out,
            "humanDenoms": human_denoms
        }

        # Send the GET request
        return requests.get(self.url + ROUTER_ROUTES_URL, params=params, headers=self.headers)

    def get_exact_amount_in_quote(self, denom_in, denom_out, human_denoms="false", singleRoute="false"):
        """
        Fetches exact amount in quote from the specified endpoint and returns it.

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
        return requests.get(self.url + ROUTER_QUOTE_URL, params=params, headers=self.headers)

    def get_exact_amount_out_quote(self, token_out, denom_in, human_denoms="false", singleRoute="false"):
        """
        Fetches exact amount out quote from the specified endpoint and returns it.

        Raises error if non-200 is returned from the endpoint.
        """

        # Set the query parameters
        params = {
            "tokenOut": token_out,
            "tokenInDenom": denom_in,
            "humanDenoms": human_denoms,
            "singleRoute": singleRoute,
        }

        print("hello from get_exact_amount_out_quote")
        print(params)

        # Send the GET request
        return requests.get(self.url + ROUTER_QUOTE_URL, params=params, headers=self.headers)

    def get_tokens_metadata(self):
        """
        Fetches tokens metadata from the specified endpoint and returns them.
        Caches them internally to avoid fetching them multiple times.

        Raises error if non-200 is returned from the endpoint.
        """
        if self.tokens_metadata:
            return self.tokens_metadata

        response = requests.get(self.url + TOKENS_METADATA_URL, headers=self.headers)

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
        response = requests.get(self.url + TOKENS_PRICES_URL, params=params, headers=self.headers)
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

    def get_canonical_orderbooks(self):
        """
        Fetches the canonical orderbooks from the specified endpoint and returns them.
        Raises error if non-200 is returned from the endpoint.
        """
        response = requests.get(self.url + CANONICAL_ORDERBOOKS_URL, headers=self.headers)

        if response.status_code != 200:
            raise Exception(f"Error fetching canonical orderbooks: {response.text}")

        return response.json()
