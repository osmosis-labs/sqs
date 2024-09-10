from locust import HttpUser, task

# burner address for the integration tests
addr = "osmo1jgz4xmaw9yk9pjxd4h8c2zs0r0vmgyn88s8t6l"


class PassthroughOrderbookActiveOrders(HttpUser):
    @task
    def passthrough_orderbook_active_orders(self):
        self.client.get(f"/passthrough/active-orders?userOsmoAddress={addr}")
