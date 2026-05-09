import unittest
from datetime import datetime

from auto_inspection.event_lifecycle import prune_history


class TestPruneHistory(unittest.TestCase):
    def test_prune_by_retention(self):
        now_dt = datetime(2026, 1, 10, 0, 0, 0)
        history = {
            "a": {"last_seen": "2026-01-01 00:00:00"},
            "b": {"last_seen": "2026-01-09 00:00:00"},
            "bad": {"last_seen": "not-a-date"},
        }

        kept, pruned = prune_history(history, now_dt, retention_days=5)
        self.assertEqual(pruned, 1)
        self.assertIn("b", kept)
        self.assertIn("bad", kept)
        self.assertNotIn("a", kept)


if __name__ == "__main__":
    unittest.main()
