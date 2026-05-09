import unittest

from auto_inspection import dashboards_bootstrap


class TestDashboardsBootstrap(unittest.TestCase):
    def test_map_osd_type(self):
        self.assertEqual(dashboards_bootstrap._map_osd_type("keyword"), "string")
        self.assertEqual(dashboards_bootstrap._map_osd_type("text"), "string")
        self.assertEqual(dashboards_bootstrap._map_osd_type("date"), "date")
        self.assertEqual(dashboards_bootstrap._map_osd_type("long"), "number")
        self.assertEqual(dashboards_bootstrap._map_osd_type("boolean"), "boolean")
        self.assertEqual(dashboards_bootstrap._map_osd_type("ip"), "ip")
        self.assertIsNone(dashboards_bootstrap._map_osd_type("object"))

    def test_saved_search_ids_are_unique(self):
        ids = [item["id"] for item in dashboards_bootstrap.SAVED_SEARCHES]
        self.assertEqual(len(ids), len(set(ids)))

    def test_data_view_ids_are_unique(self):
        ids = [item["id"] for item in dashboards_bootstrap.DATA_VIEWS]
        self.assertEqual(len(ids), len(set(ids)))

    def test_dashboard_ids_are_unique(self):
        ids = [item["id"] for item in dashboards_bootstrap.DASHBOARDS]
        self.assertEqual(len(ids), len(set(ids)))

    def test_visualization_ids_are_unique(self):
        ids = [item["id"] for item in dashboards_bootstrap.VISUALIZATIONS]
        self.assertEqual(len(ids), len(set(ids)))

    def test_dashboard_panel_refs_match_objects(self):
        search_ids = {item["id"] for item in dashboards_bootstrap.SAVED_SEARCHES}
        visualization_ids = {item["id"] for item in dashboards_bootstrap.VISUALIZATIONS}
        for dashboard in dashboards_bootstrap.DASHBOARDS:
            for panel in dashboard["panels"]:
                if panel["object_type"] == "search":
                    self.assertIn(panel["object_id"], search_ids)
                elif panel["object_type"] == "visualization":
                    self.assertIn(panel["object_id"], visualization_ids)
                else:
                    self.fail(f"Unknown panel object type: {panel['object_type']}")

    def test_incidents_objects_exist(self):
        data_view_ids = {item["id"] for item in dashboards_bootstrap.DATA_VIEWS}
        search_ids = {item["id"] for item in dashboards_bootstrap.SAVED_SEARCHES}
        visualization_ids = {item["id"] for item in dashboards_bootstrap.VISUALIZATIONS}
        self.assertIn("inspection-incidents-data-view", data_view_ids)
        self.assertIn("search-incidents-current", search_ids)
        self.assertIn("viz-incidents-oom-crashloop-trend", visualization_ids)
        self.assertIn("viz-incidents-namespace-heatmap", visualization_ids)


if __name__ == "__main__":
    unittest.main()
