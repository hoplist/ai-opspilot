import unittest

from auto_inspection import investigation_service


class TestInvestigationService(unittest.TestCase):
    def test_normalize_request_requires_target(self):
        with self.assertRaises(ValueError):
            investigation_service.normalize_request({"namespace": "default"})

    def test_normalize_request_sets_defaults(self):
        request = investigation_service.normalize_request(
            {
                "namespace": "langfuse",
                "pod": "langfuse-clickhouse-shard0-0",
            }
        )
        self.assertEqual(request["namespace"], "langfuse")
        self.assertEqual(request["pod"], "langfuse-clickhouse-shard0-0")
        self.assertLess(request["start_ts"], request["end_ts"])
        self.assertGreaterEqual(request["max_logs"], 20)

    def test_heuristic_analysis_detects_oom_crashloop(self):
        request = {
            "namespace": "langfuse",
            "pod": "langfuse-clickhouse-shard0-0",
            "workload_name": "",
            "start_ts": 1,
            "end_ts": 2,
            "max_logs": 20,
            "max_events": 20,
        }
        target = {
            "pods": [
                {
                    "namespace": "langfuse",
                    "name": "langfuse-clickhouse-shard0-0",
                    "containers": [
                        {
                            "name": "clickhouse",
                            "restart_count": 1516,
                            "state": {"kind": "waiting", "reason": "CrashLoopBackOff"},
                            "last_terminated": {"reason": "OOMKilled", "exit_code": 137},
                        }
                    ],
                }
            ]
        }
        logs = [
            {"message": "Application: Lowered mark cache size because the system has limited RAM"},
            {"message": "Address already in use"},
        ]
        events = [
            {"timestamp": "2026-04-16T12:57:34Z", "type": "Warning", "reason": "BackOff", "message": "Back-off restarting failed container"},
        ]
        prom = {
            "pods": [
                {
                    "namespace": "langfuse",
                    "pod": "langfuse-clickhouse-shard0-0",
                    "memory_limit_bytes": 402653184,
                    "memory_working_set_bytes": 360710144,
                }
            ]
        }

        analysis = investigation_service._heuristic_analysis(request, target, logs, events, prom)
        self.assertIn("memory", analysis["summary"].lower())
        self.assertTrue(analysis["root_cause"])
        self.assertIn("OOM", analysis["root_cause"][0]["title"] or "OOM")

    def test_event_target_entry_supports_non_k8s_instance(self):
        entry = investigation_service._event_target_entry(
            {
                "instance": "10.234.4.236:9100",
                "dominant_risk": "cpu",
                "final_risk_level": "high",
                "lifecycle": "ongoing",
                "runbook": {"title": "CPU 排查"},
            }
        )
        self.assertEqual(entry["instance"], "10.234.4.236:9100")
        self.assertFalse(entry["investigation_supported"])
        self.assertEqual(entry["runbook_title"], "CPU 排查")
        self.assertEqual(entry["risk_level"], "high")

    def test_merge_event_entries_does_not_double_count_score(self):
        a = investigation_service._event_target_entry(
            {
                "event_key": "instance-a:cpu",
                "instance": "instance-a",
                "dominant_risk": "cpu",
                "final_risk_level": "high",
                "lifecycle": "new",
            }
        )
        b = investigation_service._event_target_entry(
            {
                "event_key": "instance-a:cpu",
                "instance": "instance-a",
                "dominant_risk": "cpu",
                "final_risk_level": "high",
                "lifecycle": "new",
                "runbook": {"title": "CPU 排查"},
            }
        )
        merged = investigation_service._merge_event_entries(a, b)
        self.assertEqual(merged["instance"], "instance-a")
        self.assertEqual(merged["risk_level"], "high")
        self.assertEqual(merged["runbook_title"], "CPU 排查")
        self.assertGreater(merged["recommendation_score"], 0)

    def test_resolve_instance_mapping_uses_target_metadata(self):
        original_loader = investigation_service._load_active_targets
        original_workload = investigation_service._resolve_workload_from_pod
        try:
            investigation_service._load_active_targets = lambda: [
                {
                    "cluster": "kubernetes",
                    "instance": "192.168.48.201:9100",
                    "job": "kubernetes-service-endpoints",
                    "namespace": "monitoring",
                    "pod": "auto-prometheus-prometheus-node-exporter-pn5cv",
                    "service": "auto-prometheus-prometheus-node-exporter",
                    "node": "k8s-worker-1",
                    "container": "node-exporter",
                }
            ]
            investigation_service._resolve_workload_from_pod = lambda namespace, pod: {
                "workload_name": "auto-prometheus-prometheus-node-exporter",
                "workload_kind": "DaemonSet",
            }
            mapped = investigation_service._resolve_instance_mapping("192.168.48.201:9100")
        finally:
            investigation_service._load_active_targets = original_loader
            investigation_service._resolve_workload_from_pod = original_workload

        self.assertEqual(mapped["namespace"], "monitoring")
        self.assertEqual(mapped["pod"], "auto-prometheus-prometheus-node-exporter-pn5cv")
        self.assertEqual(mapped["workload_name"], "auto-prometheus-prometheus-node-exporter")
        self.assertTrue(mapped["investigation_supported"])

    def test_resolve_instance_mapping_supports_host_alias(self):
        original_loader = investigation_service._load_active_targets
        original_workload = investigation_service._resolve_workload_from_pod
        original_kube_pod_info = investigation_service._load_kube_pod_info_targets
        try:
            investigation_service._load_active_targets = lambda: [
                {
                    "cluster": "kubernetes",
                    "instance": "10.0.3.49:8080",
                    "job": "kubernetes-pods",
                    "namespace": "observability",
                    "pod": "otel-collector-5cf8f8f6d8-k4g2m",
                    "service": "otel-collector",
                    "node": "k8s-worker-1",
                    "container": "otel-collector",
                    "aliases": ["10.0.3.49:8080", "10.0.3.49", "observability/otel-collector-5cf8f8f6d8-k4g2m"],
                }
            ]
            investigation_service._load_kube_pod_info_targets = lambda: []
            investigation_service._resolve_workload_from_pod = lambda namespace, pod: {
                "workload_name": "otel-collector",
                "workload_kind": "Deployment",
            }
            mapped = investigation_service._resolve_instance_mapping("10.0.3.49")
        finally:
            investigation_service._load_active_targets = original_loader
            investigation_service._resolve_workload_from_pod = original_workload
            investigation_service._load_kube_pod_info_targets = original_kube_pod_info

        self.assertEqual(mapped["namespace"], "observability")
        self.assertEqual(mapped["pod"], "otel-collector-5cf8f8f6d8-k4g2m")
        self.assertEqual(mapped["mapped_by"], "prometheus-targets")

    def test_list_investigation_targets_prefers_incident_store(self):
        original_recent = investigation_service.list_recent_investigations
        original_incidents = investigation_service.incident_store.list_incidents
        try:
            investigation_service.list_recent_investigations = lambda limit=200: [
                {
                    "investigation_id": "inv-1",
                    "generated_at": "2026-04-20 10:00:00",
                    "namespace": "langfuse",
                    "pod": "langfuse-clickhouse-shard0-0",
                    "workload_name": "",
                    "summary": "recent investigation",
                    "logs_count": 20,
                    "events_count": 5,
                    "dashboards_links": {"overview_dashboard": "http://dash/overview"},
                }
            ]
            investigation_service.incident_store.list_incidents = lambda limit=200, include_links=True: {
                "items": [
                    {
                        "event_key": "langfuse/langfuse-clickhouse-shard0-0:mem",
                        "namespace": "langfuse",
                        "pod": "langfuse-clickhouse-shard0-0",
                    "dominant_risk": "mem",
                    "final_risk_level": "critical",
                    "lifecycle": "new",
                    "signals": ["oom", "restart"],
                    "pod_state": {
                        "restarts_total": 120,
                        "restarts": 6,
                        "waiting_reason": "CrashLoopBackOff",
                        "terminated_reason": "OOMKilled",
                        "mem_request_bytes": 268435456,
                        "mem_limit_bytes": 402653184,
                        "cpu_request_cores": 0.25,
                        "cpu_limit_cores": 0.375,
                    },
                    "links": {
                        "dashboards": {
                            "logs": "http://dash/logs",
                            "events": "http://dash/events",
                            }
                        },
                    }
                ]
            }

            items = investigation_service.list_investigation_targets(limit=10)
        finally:
            investigation_service.list_recent_investigations = original_recent
            investigation_service.incident_store.list_incidents = original_incidents

        self.assertEqual(len(items), 1)
        self.assertEqual(items[0]["namespace"], "langfuse")
        self.assertIn("investigation", items[0]["source_types"])
        self.assertIn("event", items[0]["source_types"])
        self.assertEqual(items[0]["latest_investigation_id"], "inv-1")
        self.assertEqual(items[0]["dashboards_links"]["logs"], "http://dash/logs")
        self.assertIn("oom", items[0]["signals"])
        self.assertEqual(items[0]["investigation_count"], 1)
        self.assertEqual(items[0]["waiting_reason"], "CrashLoopBackOff")
        self.assertEqual(items[0]["last_terminated_reason"], "OOMKilled")


if __name__ == "__main__":
    unittest.main()
