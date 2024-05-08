from setup import *

# DEMO - to be removed in a subsequent PR
print("Transmuter ", chain_denoms_to_display(choose_transmuter_tokens_by_liq_asc()))

print("Astroport PCL ", chain_denoms_to_display(choose_pcl_tokens_by_liq_asc()))

print("Top Liquidity Token ", chain_denoms_to_display(choose_tokens_liq_range()))

print("Top Volume Token ", chain_denoms_to_display(choose_tokens_volume_range()))

print("2 Low Liquidity Tokens ", chain_denoms_to_display(choose_tokens_liq_range(2, 5000, 10000)))

print("3 Low Volume Tokens ", chain_denoms_to_display(choose_tokens_volume_range(3, 5000, 10000)))
