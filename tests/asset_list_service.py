import requests

ASSET_LIST_URL = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/generated/frontend/assetlist.json"

class AssetListService:
    # cache the asset list, key => coinMinimalDenom, value => {symbol, decimals, coingeckoId}
    asset_map = {}

    # This is a simple service that fetches the asset list from the asset list URL
    def get_asset_metadata(self, denom):
        if self.asset_map == {}:    
            response = requests.get(ASSET_LIST_URL)
            if response.status_code != 200:
                raise Exception(f"Error fetching asset list: {response.text}")
            asset_list = response.json().get("assets", [])
            for asset in asset_list:
                coinMinimalDenom = asset.get("coinMinimalDenom")
                symbol = asset.get("symbol")
                decimals = asset.get("decimals")
                coingeckoId = asset.get("coingeckoId")
                self.asset_map[coinMinimalDenom] = {
                    "symbol": symbol,
                    "decimals": decimals,
                    "coingeckoId": coingeckoId
                }
        return self.asset_map.get(denom, None)


