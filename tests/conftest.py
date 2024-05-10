import pytest
from sqs_service import *

# Define the environment URLs
# All tests will be run against these URLs
@pytest.fixture(params=[
    SQS_STAGE,
])

def environment_url(request):
    return request.param