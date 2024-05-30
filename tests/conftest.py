import pytest
from sqs_service import *
from coingecko_service import *
import os

SERVICE_SQS_STAGE = SQSService(SQS_STAGE)
SERVICE_SQS_PROD = SQSService(SQS_PROD)
SERVICE_COINGECKO = CoingeckoService()

STAGE_INPUT_NAME = "stage"
PROD_INPUT_NAME = "prod"

# Defines the mapping between the environment input name and the SQS URL.
# E.g. stage -> SQS_STAGE
INPUT_MAP = {
    STAGE_INPUT_NAME: SQS_STAGE,
    PROD_INPUT_NAME: SQS_PROD
}

def parse_environments():
    """
    Parse the SQS_ENVIRONMENTS environment variable and return the corresponding SQS URLs

    If the environment variable is not set, the default environment is STAGE_INPUT_NAME
    """
    SQS_ENVIRONMENTS = os.getenv('SQS_ENVIRONMENTS', STAGE_INPUT_NAME)

    environments = SQS_ENVIRONMENTS.split(",")
    environment_urls = []
    for environment in environments:
        environment_url = INPUT_MAP.get(environment)
        if environment_url is None:
            raise Exception(f"Invalid environment: {environment}")

        environment_urls.append(environment_url)
    
    return environment_urls

# Define the environment URLs
# All tests will be run against these URLs
@pytest.fixture(params=parse_environments())

def environment_url(request):
    return request.param

SERVICE_MAP = {
    SQS_STAGE: SERVICE_SQS_STAGE,
    SQS_PROD: SERVICE_SQS_PROD
}
