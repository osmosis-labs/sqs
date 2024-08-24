from locust import HttpUser, task
from pairs import *
import random

# random addresses with balances
addr1 = "osmo1044qatzg4a0wm63jchrfdnn2u8nwdgxxt6e524"
addr2 = "osmo1aaa9rpq2m6tu6t0dvknqq2ps7zudxv7th209q4"
addr3 = "osmo18sd2ujv24ual9c9pshtxys6j8knh6xaek9z83t"
addr4 = "osmo140p7pef5hlkewuuramngaf5j6s8dlynth5zm06"

addresses = [addr1, addr2, addr3, addr4]

class PassthroughPortfolioBalances(HttpUser):
    @task
    def passthroughTotalCoins(self):
        random_address = random.choice(addresses)
        self.client.get(f"/passthrough/portfolio-assets/{random_address}")
