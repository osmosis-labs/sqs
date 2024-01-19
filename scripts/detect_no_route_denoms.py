# This script's purpose is to detect denoms that have a high chance of
# returning "no routes" error due to misestimating TVL.
# The script searches for pools where a specific error field is set and
# TVL is under 100 OSMO which is our current min liquidity parameter.
import json
import requests
import re

# Read JSON data from the file
with open('pools.json', 'r') as file:
    data = json.load(file)

# Create sets to store unique denominations and pools
unique_denoms = set()
unique_pools = set()

# Filter and process pools
for pool in data:
    sqs_model = pool['sqs_model']
    total_value_locked_uosmo = int(sqs_model['total_value_locked_uosmo'])
    total_value_locked_error = sqs_model.get('total_value_locked_error', '')

    # Check conditions for filtering
    # Note: add coondition to if statement below to find tokens that are likely to be excluded.
    # total_value_locked_uosmo < 100000000 and 
    if 'highest liquidity pool between base' in total_value_locked_error:
        pool_id = pool['underlying_pool']['id']
        unique_pools.add(pool_id)

        # Extract denom from the error message
        match = re.search(r"denom (\S+) not found", total_value_locked_error)
        if match:
            extracted_denom = match.group(1)
            unique_denoms.add(extracted_denom)
        else:
            print("Pattern not found in the input string.")

# Create a map of denom names
url = "https://raw.githubusercontent.com/osmosis-labs/assetlists/main/osmosis-1/osmosis-1.assetlist.json"
response = requests.get(url)

if response.status_code == 200:
    data = response.json()
    denom_to_name_map = {denom['denom']: asset['name'] for asset in data.get('assets', []) for denom in asset.get('denom_units', [])}

# Print unique denoms and pools
print("\nCount of unique denoms:", len(unique_denoms))
for denom in unique_denoms:
    name = denom_to_name_map.get(denom, "No name found")
    print(f"{denom} {name}")

print("\nCount of unique pools:", len(unique_pools))
for pool_id in unique_pools:
    print(pool_id)