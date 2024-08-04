from locust import HttpUser, task
from pairs import *

class Pools(HttpUser):
    # on_start is called when a Locust start before any task is scheduled.
    def on_start(self):
        pass

    # on_stop is called when the TaskSet is stopping
    def on_stop(self):
        pass

    # all-pools endpoint
    @task
    def all_pools(self):
        self.client.get("/pools")
    

