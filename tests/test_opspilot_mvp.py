import json
import unittest

from opspilot.core.k8s import LogRequest, _bounded_log_request, _matches_status, _pod_summary
from opspilot.core.server import route_get


class FakeK8s:
    def health(self):
        return {"mode": "fake"}

    def inventory_overview(self, limit=10):
        return {"counts": {"pod_count": 1}, "top_abnormal_pods": [], "limit": limit}

    def list_pods(self, namespace="", status="", q="", limit=100):
        return {"items": [], "item_count": 0, "total_count": 0, "truncated": False}

    def read_pod_log(self, request):
        return {"namespace": request.namespace, "pod": request.pod, "text": "hello"}

    def pod_context(self, namespace, pod):
        return {"target": {"type": "pod", "namespace": namespace, "name": pod}, "warnings": []}

    def diagnose_pod(self, namespace, pod):
        return {"target": {"type": "pod", "namespace": namespace, "name": pod}, "diagnosis": {"findings": []}}


class OpsPilotMvpTest(unittest.TestCase):
    def test_route_health(self):
        status, body = route_get("/api/health", {}, FakeK8s())
        self.assertEqual(status, 200)
        self.assertTrue(body["ok"])
        self.assertEqual(body["data"]["kubernetes"]["mode"], "fake")

    def test_route_requires_pod_log_namespace(self):
        with self.assertRaises(ValueError):
            route_get("/api/k8s/logs/pod", {"pod": "demo"}, FakeK8s())

    def test_log_request_bounds(self):
        req = _bounded_log_request(LogRequest(namespace="n", pod="p", tail_lines=99999, since_seconds=999999, limit_bytes=999999999))
        self.assertEqual(req.tail_lines, 1000)
        self.assertEqual(req.since_seconds, 86400)
        self.assertEqual(req.limit_bytes, 5 * 1024 * 1024)

    def test_pod_summary_abnormal(self):
        pod = {
            "metadata": {"namespace": "default", "name": "demo"},
            "spec": {"nodeName": "node-1"},
            "status": {
                "phase": "Running",
                "conditions": [{"type": "Ready", "status": "False"}],
                "containerStatuses": [
                    {
                        "name": "app",
                        "ready": False,
                        "restartCount": 2,
                        "state": {"waiting": {"reason": "CrashLoopBackOff"}},
                    }
                ],
            },
        }
        summary = _pod_summary(pod)
        self.assertEqual(summary["restart_count"], 2)
        self.assertTrue(_matches_status(summary, "abnormal"))
        self.assertTrue(_matches_status(summary, "crashloop"))

    def test_contract_json_is_valid(self):
        for path in [
            "opspilot/contracts/cli-schema.json",
            "opspilot/contracts/mcp-tools.json",
            "opspilot/contracts/evidence-pack.schema.json",
        ]:
            with open(path, "r", encoding="utf-8") as fh:
                self.assertIsInstance(json.load(fh), dict)


if __name__ == "__main__":
    unittest.main()
