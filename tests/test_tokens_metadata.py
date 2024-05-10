from sqs_service import *
import pytest

# Minimum number of supported tokens expected
# It should grow as we list more assets
EXPECTED_MIN_NUM_TOKENS = 250

# Tests the /tokens/metadata endpoint
class TestTokensMetadata:
    def test_token_metadata_count_above_min(self, environment_url):
        sqs_service = SQSService(environment_url)

        tokens_metadata = sqs_service.get_tokens_metadata()
        
        assert len(tokens_metadata) > EXPECTED_MIN_NUM_TOKENS, f"Token metadata count was {len(tokens_metadata)} - expected at least {EXPECTED_MIN_NUM_TOKENS}"