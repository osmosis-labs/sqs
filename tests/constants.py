import setup

# Misc constants
UOSMO = "uosmo"
USDC = 'ibc/498A0751C798A0D9A389AA3691123DADA57DAA4FE165D5C75894505B876BA6E4'

# Defaults

## Liquidity
MIN_LIQ_FILTER_DEFAULT = 5000
MAX_VAL_LOW_LIQ_FILTER_DEFAULT = 10000

## Volume
MIN_VOL_FILTER_DEFAULT = 5000
MAX_VAL_LOW_VOL_FILTER_DEFAULT = 10000
MAX_VAL_MID_VOL_FILTER_DEFAULT = 15000

## Default no. of tokens returned by helper functions
NUM_TOKENS_DEFAULT = 5

## Acceptable price differences with coingecko
HIGH_PRICE_DIFF = 0.08 ## 8%
MID_PRICE_DIFF = 0.05 ## 5%
LOW_PRICE_DIFF = 0.02 ## 2%

## Response time threshold
RT_THRESHOLD = 0.25 ## 250ms