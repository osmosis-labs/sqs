import setup
import pytest
import timeit
import time

from datetime import datetime
from sqs_service import *
from coingecko_service import *
from conftest import SERVICE_MAP
from conftest import SERVICE_COINGECKO
from constants import *

class TestTokensPrices:

    # NUM_TOKENS_DEFAULT low liquidity tokens
    @pytest.mark.parametrize("token",setup.choose_tokens_liq_range(NUM_TOKENS_DEFAULT, MIN_LIQ_FILTER_DEFAULT, MAX_VAL_LOW_LIQ_FILTER_DEFAULT))
    def test_low_liq_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, HIGH_PRICE_DIFF)

    # NUM_TOKENS_DEFAULT low volume tokens
    @pytest.mark.parametrize("token",setup.choose_tokens_volume_range(NUM_TOKENS_DEFAULT, MIN_VOL_FILTER_DEFAULT, MAX_VAL_LOW_VOL_FILTER_DEFAULT))
    def test_low_volume_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, HIGH_PRICE_DIFF)

    # NUM_TOKENS_DEFAULT mid volume tokens
    @pytest.mark.parametrize("token",setup.choose_tokens_volume_range(NUM_TOKENS_DEFAULT, MIN_VOL_FILTER_DEFAULT, MAX_VAL_MID_VOL_FILTER_DEFAULT))
    def test_mid_volume_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, MID_PRICE_DIFF)

    # NUM_TOKENS_DEFAULT top by-volume tokens
    @pytest.mark.parametrize("token", setup.choose_tokens_volume_range(NUM_TOKENS_DEFAULT))
    def test_top_volume_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, LOW_PRICE_DIFF)

    def run_coingecko_comparison_test(self, environment_url, token, price_diff_threshold):
        date_format = '%Y-%m-%d %H:%M:%S'
        sqs_service = SERVICE_MAP[environment_url]
        coingecko_service = SERVICE_COINGECKO

        # Assert coingecko id is available for the token
        coingecko_id = sqs_service.get_coingecko_id(token)
        assert coingecko_id is not None, f"{token} coingecko id is none"

        # Assert coingecko price is available for the token
        coingecko_price = coingecko_service.get_token_price(coingecko_id)
        assert coingecko_price is not None, f"{token},{coingecko_id} coingecko price is none"
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},{coingecko_id}, coingecko price = {coingecko_price}")

        # Assert the latency of the sqs pricing request is within the threshold
        measure_latency = lambda: sqs_service.get_tokens_prices(token, USDC)
        latency = timeit.timeit(measure_latency, number=1)
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},{coingecko_id}, sqs pricing response time = {latency}")
        assert latency < RT_THRESHOLD, f"{token},{coingecko_id} SQS pricing request response time exceeds {RT_THRESHOLD} second"

        # Assert sqs price is available for the token
        sqs_price_str = sqs_service.get_tokens_prices(token, USDC)
        assert sqs_price_str is not None, f"{token},{coingecko_id} SQS price is none"
        sqs_price = float(sqs_price_str)
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},${coingecko_id}, sqs price = {coingecko_price}")

        # Assert price difference between coingecko and sqs is within the threshold
        price_diff = abs(coingecko_price - sqs_price)/sqs_price 
        assert price_diff < price_diff_threshold, f"{token},{coingecko_id} price difference ({price_diff}) is greater than {price_diff_threshold}, sqs price = {sqs_price}, coingecko price = {coingecko_price}"


