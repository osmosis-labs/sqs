import requests

SQS_STAGE = "https://sqs.stage.osmosis.zone"

ROUTER_ROUTES_URL = "/router/routes"

TOKENS_METADATA_URL = "/tokens/metadata"

class SQSService:
    def __init__(self, url):
        self.url = url
        self.tokens_metadata = None

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