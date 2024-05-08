from setup import *

# DEMO - to be removed in a subsequent PR
transmuter_token_pairs = choose_transmuter_pool_tokens_by_liq_asc(1)
print("Transmuter ", [chain_denoms_to_display(pool_tokens[1]) for pool_tokens in transmuter_token_pairs])

astroport_token_pairs = choose_pcl_pool_tokens_by_liq_asc(2)
print("Astroport PCL ", [chain_denoms_to_display(pool_tokens[1]) for pool_tokens in astroport_token_pairs])

print("Top Liquidity Token ", chain_denoms_to_display(choose_tokens_liq_range()))

print("Top Volume Token ", chain_denoms_to_display(choose_tokens_volume_range()))

print("2 Low Liquidity Tokens ", chain_denoms_to_display(choose_tokens_liq_range(2, 5000, 10000)))

print("3 Low Volume Tokens ", chain_denoms_to_display(choose_tokens_volume_range(3, 5000, 10000)))
