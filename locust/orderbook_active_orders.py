from locust import HttpUser, task
from pairs import *

fill_bot_address = "osmo10s3vlv40h64qs2p98yal9w0tpm4r30uyg6ceux"

class OrderbookActiveOrders(HttpUser):
    # on_start is called when a Locust start before any task is scheduled.
    def on_start(self):
        pass

    # on_stop is called when the TaskSet is stopping
    def on_stop(self):
        pass

    
    @task
    def quoteUOSMOUSDC_1In(self):
        self.client.get(f"passthrough/active-orders?userOsmoAddress={fill_bot_address}")
