import os
import tempfile
import unittest

from auto_inspection import config
from auto_inspection import investigation_service
from auto_inspection import investigation_storage


class TestInvestigationStorage(unittest.TestCase):
    def test_sqlite_hot_store_round_trip(self):
        original_driver = config.INVESTIGATION_HOT_STORE_DRIVER
        original_path = config.INVESTIGATION_SQLITE_PATH
        try:
            with tempfile.TemporaryDirectory() as tmpdir:
                config.INVESTIGATION_HOT_STORE_DRIVER = "sqlite"
                config.INVESTIGATION_SQLITE_PATH = os.path.join(tmpdir, "hot.db")
                investigation_storage.ensure_hot_store()
                payload = {
                    "investigation_id": "inv-1",
                    "generated_at": "2026-04-21 10:00:00",
                    "request": {"namespace": "langfuse", "pod": "clickhouse-0", "workload_name": ""},
                    "analysis": {"summary": "oom suspected"},
                    "evidence": {"logs": [1, 2], "events": [1]},
                    "links": {"dashboards": {"overview_dashboard": "http://dash/overview"}},
                    "meta": {"status": "ok"},
                }
                saved = investigation_storage.save_investigation_metadata(payload, local_path="data/inv-1.json", archive={})
                self.assertTrue(saved["stored"])
                items = investigation_storage.list_recent_investigations(limit=10)
                self.assertEqual(len(items), 1)
                self.assertEqual(items[0]["investigation_id"], "inv-1")
                pointer = investigation_storage.load_investigation_pointer("latest")
                self.assertEqual(pointer["investigation_id"], "inv-1")
        finally:
            config.INVESTIGATION_HOT_STORE_DRIVER = original_driver
            config.INVESTIGATION_SQLITE_PATH = original_path

    def test_list_recent_investigations_prefers_hot_store(self):
        original_list = investigation_storage.list_recent_investigations
        try:
            investigation_storage.list_recent_investigations = lambda limit=20: [
                {
                    "investigation_id": "inv-hot",
                    "generated_at": "2026-04-21 10:00:00",
                    "namespace": "langfuse",
                    "pod": "clickhouse-0",
                    "workload_name": "",
                    "summary": "from-hot-store",
                    "logs_count": 5,
                    "events_count": 2,
                    "logs_source": "",
                    "events_source": "",
                    "use_ai": False,
                    "dashboards_links": {},
                }
            ]
            items = investigation_service.list_recent_investigations(limit=5)
        finally:
            investigation_storage.list_recent_investigations = original_list

        self.assertEqual(len(items), 1)
        self.assertEqual(items[0]["investigation_id"], "inv-hot")


if __name__ == "__main__":
    unittest.main()
