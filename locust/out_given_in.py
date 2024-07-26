from locust import HttpUser, task
from pairs import *

class ExactAmountInQuote(HttpUser):
    # on_start is called when a Locust start before any task is scheduled.
    def on_start(self):
        pass

    # on_stop is called when the TaskSet is stopping
    def on_stop(self):
        pass

    # Quote the same pair of UOSMO and USDC (UOSMO in) while progressively
    # increasing the amount of the tokenIn per endpoint.
    #
    # Token swap method is exact amount in.
    @task
    def quoteUOSMOUSDC_1In(self):
        self.client.get(f"/router/quote?tokenIn=1000000{UOSMO}&tokenOutDenom={USDC}")

    @task
    def quoteUOSMOUSDC_1000In(self):
        self.client.get(f"/router/quote?tokenIn=1000000000{UOSMO}&tokenOutDenom={USDC}")

    @task
    def quoteUOSMOUSDC_1000000In(self):
        self.client.get(f"/router/quote?tokenIn=1000000000000{UOSMO}&tokenOutDenom={USDC}")

    # Quote the same pair of UOSMO and USDC (USDC in).
    # Token swap method is exact amount in.
    @task
    def quoteUSDCUOSMO_1000000In(self):
        self.client.get(f"/router/quote?tokenIn=100000000000{USDC}&tokenOutDenom={UOSMO}")

    @task
    def quoteUSDCTUMEE_3000IN(self):
        self.client.get(f"/router/quote?tokenIn=3000000000{USDT}&tokenOutDenom={UMEE}")

    @task
    def quoteASTROCWPool(self):
        self.client.get(f"/router/quote?tokenIn=1000000000{UOSMO}&tokenOutDenom={ASTRO}")

    @task
    def quoteInvalidToken(self):
        self.client.get(f"/router/quote?tokenIn=1000000000{UOSMO}&tokenOutDenom={INVALID_DENOM}")

    @task
    def routesUOSMOUSDC(self):
        self.client.get(f"/router/routes?tokenIn={UOSMO}&tokenOutDenom={USDC}")

    @task
    def routesUSDCUOSMO(self):
        self.client.get(f"/router/routes?tokenIn={USDC}&tokenOutDenom={UOSMO}")

