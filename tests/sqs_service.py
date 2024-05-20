import requests

SQS_STAGE = "https://sqs.stage.osmosis.zone"
SQS_PROD = "https://sqs.osmosis.zone"

ROUTER_ROUTES_URL = "/router/routes"

TOKENS_METADATA_URL = "/tokens/metadata"

CONFIG_URL = "/config"

class SQSService:
    def __init__(self, url):
        self.url = url
        self.tokens_metadata = None
        self.config = None

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