import requests

from unittest import TestCase

class ProxyTesting(TestCase):
    def test_proxy(self):
        r = requests.get("https://ipinfo.io")
        print(r)
