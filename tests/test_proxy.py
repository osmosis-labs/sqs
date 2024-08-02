import requests

from unittest import TestCase

class ProxyTesting(TestCase):
    def test_proxy(self):
        response = requests.get("https://ipinfo.io")
        self.assertEqual(response.status_code, 200)
        self.assertIn("ip", response.json())
