import unittest

from auto_inspection import dashboard_server


class TestDashboardServer(unittest.TestCase):
    def test_normalize_pipeline_request_supports_lists(self):
        request = dashboard_server._normalize_pipeline_request(
            {
                "steps": ["targets", "baseline"],
                "skip": "report",
                "continue_on_error": True,
            }
        )
        self.assertEqual(request["steps"], ["targets", "baseline"])
        self.assertEqual(request["skip"], ["report"])
        self.assertTrue(request["continue_on_error"])

    def test_normalize_pipeline_request_rejects_mixed_selection(self):
        with self.assertRaises(ValueError):
            dashboard_server._normalize_pipeline_request(
                {
                    "steps": "targets,baseline",
                    "from_step": "targets",
                }
            )

    def test_normalize_targets_supports_multiline_and_csv(self):
        targets = dashboard_server._normalize_targets("prod/default/api-0,default/worker-0\r\nsolo-pod")
        self.assertEqual(
            targets,
            ["prod/default/api-0", "default/worker-0", "solo-pod"],
        )

    def test_normalize_notification_settings_sanitizes_fields(self):
        settings = dashboard_server._normalize_notification_settings(
            {
                "enabled": True,
                "webhook_url": " https://example.com/hook ",
                "webhook_type": "unknown",
                "targets": "default/api-0\nworker-0",
                "state_file": "",
            }
        )
        self.assertTrue(settings["enabled"])
        self.assertEqual(settings["webhook_url"], "https://example.com/hook")
        self.assertEqual(settings["webhook_type"], "generic")
        self.assertEqual(settings["targets"], ["default/api-0", "worker-0"])
        self.assertEqual(settings["state_file"], "data/pod_restart_notify_state.json")

    def test_normalize_link_templates_trims_known_keys(self):
        links = dashboard_server._normalize_link_templates(
            {
                "logs": " https://logs.example/{pod} ",
                "events": " https://events.example/{pod} ",
                "yaml": "",
                "shell": "  ",
                "metrics": " https://prom.example/{expr_url} ",
                "unknown": "ignored",
            }
        )
        self.assertEqual(
            links,
            {
                "logs": "https://logs.example/{pod}",
                "events": "https://events.example/{pod}",
                "yaml": "",
                "shell": "",
                "metrics": "https://prom.example/{expr_url}",
            },
        )

    def test_normalize_dashboard_settings_merges_notification_and_links(self):
        settings = dashboard_server._normalize_dashboard_settings(
            {
                "notification": {
                    "enabled": True,
                    "webhook_url": " https://example.com/hook ",
                    "webhook_type": "wecom",
                    "targets": "prod/default/api-0",
                },
                "links": {
                    "logs": " https://logs.example/{pod} ",
                    "metrics": " https://prom.example/{expr_url} ",
                },
            }
        )
        self.assertTrue(settings["notification"]["enabled"])
        self.assertEqual(settings["notification"]["webhook_url"], "https://example.com/hook")
        self.assertEqual(settings["notification"]["webhook_type"], "wecom")
        self.assertEqual(settings["notification"]["targets"], ["prod/default/api-0"])
        self.assertEqual(settings["links"]["logs"], "https://logs.example/{pod}")
        self.assertEqual(settings["links"]["metrics"], "https://prom.example/{expr_url}")
        self.assertIn("events", settings["links"])
        self.assertIn("yaml", settings["links"])
        self.assertIn("shell", settings["links"])


if __name__ == "__main__":
    unittest.main()
