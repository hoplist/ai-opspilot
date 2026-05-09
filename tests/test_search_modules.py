import unittest

from auto_inspection import event_search
from auto_inspection import log_search


class TestLogSearch(unittest.TestCase):
    def test_build_log_query_includes_text_filters_and_range(self):
        query = log_search.build_log_query(
            {
                "q": "error timeout",
                "cluster": "prod-a",
                "namespace": "payments",
                "pod": "api-0",
                "severity": "error",
                "start_ts": 1713200000,
                "end_ts": 1713203600,
            }
        )

        query_text = query["bool"]["must"][0]["simple_query_string"]["query"]
        self.assertEqual(query_text, "error timeout")
        filters = query["bool"]["filter"]
        self.assertTrue(any("range" in item for item in filters))
        cluster_filters = [item for item in filters if "bool" in item]
        self.assertTrue(
            any(
                any(
                    term.get("term") in ({"cluster": "prod-a"}, {"cluster.keyword": "prod-a"})
                    for term in item["bool"]["should"]
                )
                for item in cluster_filters
            )
        )

    def test_normalize_log_hit_prefers_kubernetes_fields_when_needed(self):
        item = log_search.normalize_log_hit(
            {
                "_id": "1",
                "_index": "logs-k8s-2026.04.16",
                "_score": 1.0,
                "_source": {
                    "@timestamp": "2026-04-16T10:00:00Z",
                    "message": "panic: timeout",
                    "message_normalized": "panic: timeout",
                    "severity": "error",
                    "logger": "app.main",
                    "stack_language": "go",
                    "exception_type": "panic",
                    "exception_message": "timeout",
                    "log": {"level": "error", "logger": "app.main"},
                    "kubernetes": {
                        "namespace_name": "default",
                        "pod_name": "api-0",
                        "container_name": "api",
                        "node_name": "node-a",
                    },
                },
            }
        )
        self.assertEqual(item["namespace"], "default")
        self.assertEqual(item["pod"], "api-0")
        self.assertEqual(item["container"], "api")
        self.assertEqual(item["severity"], "error")
        self.assertEqual(item["exception_type"], "panic")
        self.assertEqual(item["stack_language"], "go")


class TestEventSearch(unittest.TestCase):
    def test_build_event_query_includes_reason_and_range(self):
        query = event_search.build_event_query(
            {
                "q": "crashloop",
                "cluster": "prod-a",
                "namespace": "default",
                "reason": "BackOff",
                "type": "Warning",
                "start_ts": 1713200000,
                "end_ts": 1713203600,
            }
        )

        query_text = query["bool"]["must"][0]["simple_query_string"]["query"]
        self.assertEqual(query_text, "crashloop")
        filters = query["bool"]["filter"]
        self.assertTrue(any("range" in item for item in filters))
        bool_filters = [item for item in filters if "bool" in item]
        self.assertTrue(
            any(
                any(
                    term.get("term") in ({"reason": "BackOff"}, {"reason.keyword": "BackOff"})
                    for term in item["bool"]["should"]
                )
                for item in bool_filters
            )
        )
        self.assertTrue(
            any(
                any(
                    term.get("term") in ({"type": "Warning"}, {"type.keyword": "Warning"})
                    for term in item["bool"]["should"]
                )
                for item in bool_filters
            )
        )

    def test_normalize_event_hit_keeps_regarding_context(self):
        item = event_search.normalize_event_hit(
            {
                "_id": "evt-1",
                "_index": "events-k8s-2026.04.16",
                "_score": 1.0,
                "_source": {
                    "@timestamp": "2026-04-16T10:00:00Z",
                    "cluster": "prod-a",
                    "reason": "BackOff",
                    "type": "Warning",
                    "note": "Back-off restarting failed container",
                    "regarding": {
                        "kind": "Pod",
                        "name": "api-0",
                        "namespace": "default",
                    },
                },
            }
        )
        self.assertEqual(item["namespace"], "default")
        self.assertEqual(item["message"], "Back-off restarting failed container")
        self.assertEqual(item["regarding"]["name"], "api-0")
