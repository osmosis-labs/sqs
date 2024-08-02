import requests
import os
from unittest import TestCase

class ProxyTesting(TestCase):
    def test_proxy(self):
        response = requests.get("https://ipinfo.io")
        self.assertEqual(response.status_code, 200)
        self.assertIn("region", response.json())
        region = response.json()['region']
        os.system(f'echo Runing in the region: "{region}" >> $GITHUB_STEP_SUMMARY')
