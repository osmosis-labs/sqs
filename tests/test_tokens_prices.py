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
        self.run_coingecko_comparison_test(environment_url, token, HIGH_PRICE_DIFF, True)

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

    # NUM_TOKENS_DEFAULT top by-volume tokens in a batch request, in which multiple tokens
    # are requested in a single request to /tokens/prices
    def test_top_volume_token_prices_in_batch(self, environment_url):
        tokens = setup.choose_tokens_volume_range(NUM_TOKENS_DEFAULT)
        date_format = '%Y-%m-%d %H:%M:%S'
        sqs_service = SERVICE_MAP[environment_url]
        # Assert the latency of the sqs pricing request is within the threshold
        measure_latency = lambda: sqs_service.get_tokens_prices(tokens)
        latency = timeit.timeit(measure_latency, number=1)
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: sqs pricing response time = {latency}")
        assert latency < RT_THRESHOLD, f"SQS pricing request response time {latency} exceeds {RT_THRESHOLD} second"

        # Assert sqs price is available for the token
        sqs_price_json = sqs_service.get_tokens_prices(tokens)
        for token in tokens:
            sqs_price_str = sqs_price_json.get(token, {}).get(USDC, None)
            assert sqs_price_str is not None, f"{token} SQS price is none"
            sqs_price = float(sqs_price_str)
            assert sqs_price > 0, f"{token} SQS price is zero"

    # Test every valid listed token if it is supported by the /tokens/prices endpoint
    # Count the number of unsupported tokens and assert it is below the threshold
    def test_unsupported_token_count(self, environment_url):
        tokens = setup.choose_valid_listed_tokens()
        sqs_service = SERVICE_MAP[environment_url]
        supported_token_count = 0
        unsupported_token_count = 0
        for token in tokens:
            try:
                sqs_price_json = sqs_service.get_tokens_prices([token])
            except Exception as e:
                # Increment unsupported token count if an exception is raised
                unsupported_token_count += 1
                f"Unsupported token {token}: error fetching sqs price {str(e)}"
            sqs_price_str = sqs_price_json.get(token, {}).get(USDC, None)
            if sqs_price_str is None:
                unsupported_token_count += 1
                # Increment unsupported token count if the price is not available
                f"Unsupported token {token}: SQS price is none in response"
            sqs_price = float(sqs_price_str)
            if sqs_price <= 0:
                unsupported_token_count += 1
                # Increment unsupported token count if the price is zero
                f"Unsupported token {token}: SQS price is zero"
            else:
                supported_token_count += 1
        assert unsupported_token_count <= UNSUPPORTED_TOKEN_COUNT_THRESHOLD, f"Unsupported token count: {unsupported_token_count} exceeds threshold {UNSUPPORTED_TOKEN_COUNT_THRESHOLD}"

    # Helper function to run the coingecko/sqs price comparison test
    # The following tests are performed given the token denom
    # 1. Test if its corresponding coingecko id and coingecko price is available
    # 2. Test if its sqs request latency is within the threshold
    # 3. Test if its sqs price is available
    # 4. Test if the price difference between coingecko and sqs is within the threshold
    def run_coingecko_comparison_test(self, environment_url, token, price_diff_threshold, allow_blank_coingecko_id=False):
        date_format = '%Y-%m-%d %H:%M:%S'
        sqs_service = SERVICE_MAP[environment_url]
        coingecko_service = SERVICE_COINGECKO

        # Assert coingecko id is available for the token
        coingecko_id = sqs_service.get_coingecko_id(token)
        if (not allow_blank_coingecko_id):
            assert coingecko_id is not None, f"{token} coingecko id is none"
        else:
            return

        # Assert coingecko price is available for the token
        coingecko_price = coingecko_service.get_token_price(coingecko_id)
        assert coingecko_price is not None, f"{token},{coingecko_id} coingecko price is none"
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},{coingecko_id}, coingecko price = {coingecko_price}")
        assert coingecko_price > 0, f"{token},{coingecko_id} coingecko price is zero"

        # Assert the latency of the sqs pricing request is within the threshold
        measure_latency = lambda: sqs_service.get_tokens_prices([token])
        latency = timeit.timeit(measure_latency, number=1)
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},{coingecko_id}, sqs pricing response time = {latency}")
        assert latency < RT_THRESHOLD, f"{token},{coingecko_id} SQS pricing request response time {latency} exceeds {RT_THRESHOLD} second"

        # Assert sqs price is available for the token
        sqs_price_json = sqs_service.get_tokens_prices([token])
        sqs_price_str = sqs_price_json.get(token, {}).get(USDC, None)
        assert sqs_price_str is not None, f"{token},{coingecko_id} SQS price is none"
        sqs_price = float(sqs_price_str)
        assert sqs_price > 0, f"{token},{coingecko_id} SQS price is zero"
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},${coingecko_id}, sqs price = {coingecko_price}")

        # Assert price difference between coingecko and sqs is within the threshold
        price_diff = abs(coingecko_price - sqs_price)/sqs_price 
        assert price_diff < price_diff_threshold, f"{token},{coingecko_id} price difference ({price_diff}) is greater than {price_diff_threshold}, sqs price = {sqs_price}, coingecko price = {coingecko_price}"


