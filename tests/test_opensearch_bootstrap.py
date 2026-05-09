import unittest

from auto_inspection import opensearch_bootstrap


class TestOpenSearchBootstrap(unittest.TestCase):
    def test_logs_template_contains_normalized_fields(self):
        template = opensearch_bootstrap._logs_template()
        props = template["template"]["mappings"]["properties"]
        self.assertIn("message_normalized", props)
        self.assertIn("exception_type", props)
        self.assertIn("exception_message", props)
        self.assertIn("stack_language", props)

    def test_retention_policies_cover_all_indices(self):
        policies = opensearch_bootstrap._retention_policies()
        self.assertEqual(set(policies.keys()), {"logs", "events", "incidents", "investigations"})
        self.assertEqual(
            policies["logs"]["policy"]["states"][0]["transitions"][0]["conditions"]["min_index_age"],
            "14d",
        )

    def test_snapshot_repository_uses_fs_type(self):
        repo = opensearch_bootstrap._snapshot_repository()
        self.assertEqual(repo["type"], "fs")
        self.assertIn("/snapshots", repo["settings"]["location"])


if __name__ == "__main__":
    unittest.main()
