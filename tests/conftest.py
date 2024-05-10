import pytest
from sqs_service import *

SERVICE_SQS_STAGE = SQSService(SQS_STAGE)
SERVICE_SQS_PROD = SQSService(SQS_PROD)

# Define the environment URLs
# All tests will be run against these URLs
@pytest.fixture(params=[
    SQS_STAGE,
])

def environment_url(request):
    return request.param

SERVICE_MAP = {
    SQS_STAGE: SERVICE_SQS_STAGE,
    SQS_PROD: SERVICE_SQS_PROD
}
