import conftest
import pytest

from sqs_service import *
import util
from conftest import SERVICE_MAP
from e2e_math import *
from decimal import *

# Test suite for the /passthrough endpoint

# Note: this is for convinience to skip long-running tests in development
# locally.
# @pytest.mark.skip(reason="This test is currently disabled")
class TestPassthrough:

    def test_poortfolio_assets(self, environment_url):
        sqs_service = SERVICE_MAP[environment_url]
        
        # Arbitrary addresses
        addresses = [
            "osmo1044qatzg4a0wm63jchrfdnn2u8nwdgxxt6e524",
            "osmo1aaa9rpq2m6tu6t0dvknqq2ps7zudxv7th209q4",
            "osmo18sd2ujv24ual9c9pshtxys6j8knh6xaek9z83t",
            "osmo140p7pef5hlkewuuramngaf5j6s8dlynth5zm06",
        ]

        for address in addresses:
            response = sqs_service.get_portfolio_assets(address)

            totalValueCap = response.get('total_value_cap')
            accountCoinsResult = response.get('account_coins_result')

            print(address)
            print(f"totalValueCap: {totalValueCap}")
            print(f"accountCoinsResult: {accountCoinsResult}")

            assert totalValueCap is not None
            assert accountCoinsResult is not None

            assert Decimal(totalValueCap) >= 0
            assert len(accountCoinsResult) > 0


