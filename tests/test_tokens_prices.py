import pytest
import timeit
import time
import os

from datetime import datetime
from sqs_service import *
from coingecko_service import *
import conftest 
from constants import *
from filelock import FileLock

counter_file = "/tmp/counter.txt"
lock_file = "/tmp/counter.lock"

class TestTokensPrices:

    # Initialize the counter file at the beginning of the test
    def setup_class(cls):
        if os.path.exists(counter_file):
            os.remove(counter_file)
        cls().write_counter(0)

    # Assert that the unsupported token count is within the threshold
    # Clean up the counter file at the end of the test
    def teardown_class(cls):
        unsupported_token_count = cls().read_counter()
        if os.path.exists(counter_file):
            os.remove(counter_file)
        assert unsupported_token_count <= UNSUPPORTED_TOKEN_COUNT_THRESHOLD, f"Unsupported token count: {unsupported_token_count} exceeds threshold {UNSUPPORTED_TOKEN_COUNT_THRESHOLD}"

    # Function to read the current counter value
    def read_counter(self):
        if not os.path.exists(counter_file):
            return 0
        with open(counter_file, "r") as file:
            return int(file.read().strip())

    # Function to write the new counter value
    def write_counter(self, value):
        with open(counter_file, "w") as file:
            file.write(str(value))

    # Function to safely increment the counter
    def increment_counter(self):
        with FileLock(lock_file):
            counter = self.read_counter()
            counter += 1
            self.write_counter(counter)

    # NUM_TOKENS_DEFAULT low liquidity tokens
    @pytest.mark.parametrize("token",conftest.choose_tokens_liq_range(NUM_TOKENS_DEFAULT, MIN_LIQ_FILTER_DEFAULT, MAX_VAL_LOW_LIQ_FILTER_DEFAULT))
    def test_low_liq_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, HIGH_PRICE_DIFF, allow_blank_coingecko_id=True)

    # NUM_TOKENS_DEFAULT low volume tokens
    @pytest.mark.parametrize("token",conftest.choose_tokens_volume_range(NUM_TOKENS_DEFAULT, MIN_VOL_FILTER_DEFAULT, MAX_VAL_LOW_VOL_FILTER_DEFAULT))
    def test_low_volume_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, HIGH_PRICE_DIFF, allow_blank_coingecko_id=True)

    # NUM_TOKENS_DEFAULT mid volume tokens
    @pytest.mark.parametrize("token",conftest.choose_tokens_volume_range(NUM_TOKENS_DEFAULT, MIN_VOL_FILTER_DEFAULT, MAX_VAL_MID_VOL_FILTER_DEFAULT))
    def test_mid_volume_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, MID_PRICE_DIFF, allow_blank_coingecko_id=True)

    # NUM_TOKENS_DEFAULT top by-volume tokens
    @pytest.mark.parametrize("token", conftest.choose_tokens_volume_range(NUM_TOKENS_DEFAULT))
    def test_top_volume_token_prices(self, environment_url, token):
        self.run_coingecko_comparison_test(environment_url, token, LOW_PRICE_DIFF, allow_blank_coingecko_id=False)

    # Test every valid listed token if it is supported by the /tokens/prices endpoint
    # Tests are run by separate processes in parallel, thus using the filelock
    # to ensure that the counter is updated safely 
    @pytest.mark.parametrize("token", conftest.shared_test_state.valid_listed_tokens)
    def test_unsupported_token_count(self, environment_url, token):
        sqs_service = conftest.SERVICE_MAP[environment_url]
        try:
            sqs_price_json = sqs_service.get_tokens_prices([token])
        except Exception as e:
            # Increment unsupported token count if an exception is raised
            self.increment_counter()
            f"Unsupported token {token}: error fetching sqs price {str(e)}"
        sqs_price_str = sqs_price_json.get(token, {}).get(USDC, None)
        if sqs_price_str is None:
            self.increment_counter()
            # Increment unsupported token count if the price is not available
            f"Unsupported token {token}: SQS price is none in response"
        sqs_price = float(sqs_price_str)
        if sqs_price <= 0:
            self.increment_counter()
            # Increment unsupported token count if the price is zero
            f"Unsupported token {token}: SQS price is zero"

    # NUM_TOKENS_DEFAULT top by-volume tokens in a batch request, in which multiple tokens
    # are requested in a single request to /tokens/prices
    def test_top_volume_token_prices_in_batch(self, environment_url):
        tokens = conftest.choose_tokens_volume_range(NUM_TOKENS_DEFAULT)
        sqs_service = conftest.SERVICE_MAP[environment_url]
        # Assert the latency of the sqs pricing request is within the threshold
        measure_latency = lambda: sqs_service.get_tokens_prices(tokens)
        latency = timeit.timeit(measure_latency, number=1)
        assert latency < RT_THRESHOLD, f"SQS pricing request response time {latency} exceeds {RT_THRESHOLD} second"

        # Assert sqs price is available for the token
        sqs_price_json = sqs_service.get_tokens_prices(tokens)
        for token in tokens:
            sqs_price_str = sqs_price_json.get(token, {}).get(USDC, None)
            assert sqs_price_str is not None, f"{token} SQS price is none"
            sqs_price = float(sqs_price_str)
            assert sqs_price > 0, f"{token} SQS price is zero"

    # Helper function to run the coingecko/sqs price comparison test
    # The following tests are performed given the token denom
    # 1. Test if its corresponding coingecko id and coingecko price is available
    # 2. Test if its sqs request latency is within the threshold
    # 3. Test if its sqs price is available
    # 4. Test if the price difference between coingecko and sqs is within the threshold
    def run_coingecko_comparison_test(self, environment_url, token, price_diff_threshold, allow_blank_coingecko_id=False):
        date_format = '%Y-%m-%d %H:%M:%S'
        sqs_service = conftest.SERVICE_MAP[environment_url]

        # Assert the latency of the sqs pricing request is within the threshold
        measure_latency = lambda: sqs_service.get_tokens_prices([token])
        latency = timeit.timeit(measure_latency, number=1)
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},, sqs pricing response time = {latency}")
        assert latency < RT_THRESHOLD, f"{token}, SQS pricing request response time {latency} exceeds {RT_THRESHOLD} second"

        # Assert sqs price is available for the token
        sqs_price_json = sqs_service.get_tokens_prices([token])
        sqs_price_str = sqs_price_json.get(token, {}).get(USDC, None)
        assert sqs_price_str is not None, f"{token},s SQS price is none"
        sqs_price = float(sqs_price_str)
        assert sqs_price > 0, f"{token}, SQS price is zero"
        print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token}, sqs price = {sqs_price}")

        # Assert coingecko id is available for the token if it is not allowed to be blank
        coingecko_id = sqs_service.get_coingecko_id(token)
        assert allow_blank_coingecko_id or coingecko_id is not None, f"{token}, coingecko id is none"

        # If coingecko id is available, perform the price comparison against its price
        if coingecko_id is not None and not allow_blank_coingecko_id:
            coingecko_service = conftest.SERVICE_COINGECKO
            # Assert coingecko price is available for the token
            coingecko_price = coingecko_service.get_token_price(coingecko_id)
            assert coingecko_price is not None, f"{token},{coingecko_id} coingecko price is none"
            print(f"{datetime.fromtimestamp(time.time()).strftime(date_format)}: {token},{coingecko_id}, coingecko price = {coingecko_price}")
            assert coingecko_price > 0, f"{token},{coingecko_id} coingecko price is zero"

            # Assert price difference between coingecko and sqs is within the threshold
            price_diff = abs(coingecko_price - sqs_price)/sqs_price 
            assert price_diff < price_diff_threshold, f"{token},{coingecko_id} price difference ({price_diff}) is greater than {price_diff_threshold}, sqs price = {sqs_price}, coingecko price = {coingecko_price}"


