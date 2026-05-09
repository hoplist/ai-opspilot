import unittest

from auto_inspection import config
from auto_inspection import source_context


class TestSourceContext(unittest.TestCase):
    def test_source_fingerprint_prefers_source_id_over_urls(self):
        original_source_id = config.SOURCE_ID
        original_urls = config.PROMETHEUS_URLS
        original_clusters = config.PROMETHEUS_CLUSTERS
        try:
            config.SOURCE_ID = "shared-kubernetes-observability"
            config.PROMETHEUS_CLUSTERS = ["kubernetes"]
            config.PROMETHEUS_URLS = ["http://prom-a:9090"]
            first = source_context.source_fingerprint()
            config.PROMETHEUS_URLS = ["http://prom-b:9090"]
            second = source_context.source_fingerprint()
        finally:
            config.SOURCE_ID = original_source_id
            config.PROMETHEUS_URLS = original_urls
            config.PROMETHEUS_CLUSTERS = original_clusters

        self.assertEqual(first, second)


if __name__ == "__main__":
    unittest.main()
