from locust import HttpUser, task
from pairs import *

class ExactAmountOutQuote(HttpUser):
    # on_start is called when a Locust start before any task is scheduled.
    def on_start(self):
        pass

    # on_stop is called when the TaskSet is stopping
    def on_stop(self):
        pass

    # Quote the same pair of UOSMO and USDC (UOSMO in) while progressively
    # increasing the amount of the tokenIn per endpoint.
    #
    # Token swap method is exact amount out.
    @task
    def quoteUOSMOUSDC_1In(self):
        self.client.get(f"/router/quote?tokenOut=1000000{UOSMO}&tokenInDenom={USDC}")

    @task
    def quoteUOSMOUSDC_1000In(self):
        self.client.get(f"/router/quote?tokenOut=1000000000{UOSMO}&tokenInDenom={USDC}")

    @task
    def quoteUOSMOUSDC_1000000In(self):
        self.client.get(f"/router/quote?tokenOut=1000000000000{UOSMO}&tokenInDenom={USDC}")

    # Quote the same pair of UOSMO and USDC (USDC in).
    # Token swap method is exact amount out.
    @task
    def quoteUSDCUOSMO_1000000In(self):
        self.client.get(f"/router/quote?tokenOut=100000000000{USDC}&tokenInDenom={UOSMO}")

    @task
    def quoteUSDCTUMEE_3000IN(self):
        self.client.get(f"/router/quote?tokenOut=3000000000{USDT}&tokenInDenom={UMEE}")

    @task
    def quoteASTROCWPool(self):
        self.client.get(f"/router/quote?tokenOut=1000000000{UOSMO}&tokenInDenom={ASTRO}")

    @task
    def quoteInvalidToken(self):
        self.client.get(f"/router/quote?tokenOut=1000000000{UOSMO}&tokenInDenom={INVALID_DENOM}")
