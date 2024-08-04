from locust import HttpUser, task
from pairs import *

class TokenPrices(HttpUser):
    # on_start is called when a Locust start before any task is scheduled.
    def on_start(self):
        pass

    # on_stop is called when the TaskSet is stopping
    def on_stop(self):
        pass

    @task
    def tokenPrices(self):
        bases = ""
        for i in range(len(top10ByVolumePairs)):
            bases += top10ByVolumePairs[i]
            if i < len(top10ByVolumePairs) - 1:
                bases += ","

        self.client.get(f"/tokens/prices?base={bases}")
