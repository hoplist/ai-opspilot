import unittest

from auto_inspection import incident_store


class TestIncidentStore(unittest.TestCase):
    def test_search_incidents_local_cache_enriches_items(self):
        original_is_configured = incident_store.opensearch_client.is_configured
        original_local_events = incident_store._list_local_events
        original_dashboards = incident_store._incident_dashboards_links
        try:
            incident_store.opensearch_client.is_configured = lambda: False
            incident_store._list_local_events = lambda limit=20, include_links=True: [
                {
                    "namespace": "langfuse",
                    "pod": "langfuse-clickhouse-shard0-0",
                    "dominant_risk": "mem",
                    "final_risk_level": "critical",
                }
            ]
            incident_store._incident_dashboards_links = lambda item: {
                "logs": "http://dash/logs",
                "events": "http://dash/events",
            }

            payload = incident_store.search_incidents(q="mem", limit=10)
        finally:
            incident_store.opensearch_client.is_configured = original_is_configured
            incident_store._list_local_events = original_local_events
            incident_store._incident_dashboards_links = original_dashboards

        self.assertEqual(payload["source"], "local-json-cache")
        self.assertEqual(len(payload["items"]), 1)
        self.assertTrue(payload["items"][0]["investigation_supported"])
        self.assertEqual(payload["items"][0]["links"]["dashboards"]["logs"], "http://dash/logs")

    def test_list_incidents_uses_current_source_filter_and_sort(self):
        original_is_configured = incident_store.opensearch_client.is_configured
        original_search = incident_store.opensearch_client.search
        original_hits = incident_store.opensearch_client.response_hits
        original_dashboards = incident_store._incident_dashboards_links
        calls = {}
        try:
            incident_store.opensearch_client.is_configured = lambda: True

            def fake_search(index, query, *, size=50, from_=0, sort=None, source_includes=None, timeout=None):
                calls["index"] = index
                calls["query"] = query
                calls["size"] = size
                calls["sort"] = sort
                return {
                    "hits": {
                        "hits": [
                            {
                                "_source": {
                                    "namespace": "langfuse",
                                    "pod": "langfuse-clickhouse-shard0-0",
                                    "final_risk_level": "critical",
                                }
                            }
                        ]
                    }
                }

            incident_store.opensearch_client.search = fake_search
            incident_store.opensearch_client.response_hits = lambda payload: (1, payload["hits"]["hits"])
            incident_store._incident_dashboards_links = lambda item: {}

            payload = incident_store.list_incidents(limit=3)
        finally:
            incident_store.opensearch_client.is_configured = original_is_configured
            incident_store.opensearch_client.search = original_search
            incident_store.opensearch_client.response_hits = original_hits
            incident_store._incident_dashboards_links = original_dashboards

        self.assertEqual(payload["source"], "opensearch")
        self.assertEqual(calls["size"], 3)
        self.assertEqual(
            calls["query"]["bool"]["filter"][0]["term"]["source.fingerprint"],
            incident_store.source_context.source_fingerprint(),
        )
        self.assertEqual(calls["sort"][0]["risk_score"]["order"], "desc")


if __name__ == "__main__":
    unittest.main()
