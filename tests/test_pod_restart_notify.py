import unittest

from auto_inspection import pod_restart_notify


def _pod(
    cluster="prod",
    namespace="default",
    pod="api-0",
    restarts_total=0,
    restarts=0,
):
    return {
        "cluster": cluster,
        "namespace": namespace,
        "pod": pod,
        "instance": f"{namespace}/{pod}",
        "node_name": "node-a",
        "phase": "Running",
        "pod_status": "Running",
        "terminated_reason": "",
        "terminated_exitcode": None,
        "last_terminated_time": None,
        "restart_window_hours": 24,
        "restarts_total": restarts_total,
        "restarts": restarts,
    }


class TestPodRestartNotify(unittest.TestCase):
    def test_match_targets_from_pod_states(self):
        resource_data = {
            "pod_states": [
                {
                    "cluster": "prod",
                    "namespace": "default",
                    "pod": "api-0",
                    "instance": "default/api-0",
                    "node_name": "node-a",
                    "phase": "Running",
                    "pod_status": "Running",
                    "restarts": 1,
                    "restarts_total": 3,
                    "restart_window_hours": 24,
                }
            ]
        }

        matched = pod_restart_notify.extract_monitored_pods(
            resource_data,
            start_ts=0,
            end_ts=1,
            targets=["default/api-0"],
        )
        self.assertEqual(len(matched), 1)
        self.assertEqual(matched[0]["cluster"], "prod")

    def test_match_targets(self):
        resource_data = {
            "entries": [
                {
                    "key": "pod_cpu",
                    "instance": "default/api-0",
                    "labels": {"cluster": "prod", "namespace": "default", "pod": "api-0"},
                    "group_label": "cluster=prod namespace=default",
                    "current": 0.3,
                    "period_max": 0.4,
                    "oom": False,
                    "phase": "Running",
                    "pod_status": "Running",
                    "restarts": 1,
                    "restarts_total": 3,
                    "restart_window_hours": 24,
                },
                {
                    "key": "pod_mem",
                    "instance": "default/api-0",
                    "labels": {"cluster": "prod", "namespace": "default", "pod": "api-0"},
                    "group_label": "cluster=prod namespace=default",
                    "current": 0.5,
                    "period_max": 0.6,
                    "oom": False,
                    "phase": "Running",
                    "pod_status": "Running",
                    "restarts": 1,
                    "restarts_total": 3,
                    "restart_window_hours": 24,
                },
            ],
            "trend_enabled": False,
            "trend_days": 7,
        }

        matched = pod_restart_notify.extract_monitored_pods(
            resource_data,
            start_ts=0,
            end_ts=1,
            targets=["prod/default/api-0"],
        )
        self.assertEqual(len(matched), 1)
        self.assertEqual(matched[0]["pod"], "api-0")

    def test_first_seen_only_seeds_state(self):
        events, state = pod_restart_notify.detect_restart_events(
            [_pod(restarts_total=2, restarts=1)],
            {"pods": {}},
            "2026-03-10 12:00:00",
        )
        self.assertEqual(events, [])
        state_key = "prod|default|api-0"
        self.assertEqual(state["pods"][state_key]["last_notified_total"], 2)

    def test_restart_delta_triggers_event(self):
        events, state = pod_restart_notify.detect_restart_events(
            [_pod(restarts_total=5, restarts=2)],
            {
                "pods": {
                    "prod|default|api-0": {
                        "last_notified_total": 3,
                    }
                }
            },
            "2026-03-10 12:00:00",
        )
        self.assertEqual(len(events), 1)
        self.assertEqual(events[0]["restart_delta"], 2)
        self.assertEqual(state["pods"]["prod|default|api-0"]["last_notified_total"], 3)

    def test_counter_reset_uses_recent_restarts(self):
        events, _ = pod_restart_notify.detect_restart_events(
            [_pod(restarts_total=1, restarts=1)],
            {
                "pods": {
                    "prod|default|api-0": {
                        "last_notified_total": 7,
                    }
                }
            },
            "2026-03-10 12:00:00",
        )
        self.assertEqual(len(events), 1)
        self.assertTrue(events[0]["counter_reset"])
        self.assertEqual(events[0]["restart_delta"], 1)


if __name__ == "__main__":
    unittest.main()
