import unittest

from auto_inspection import stats


class TestPercentile(unittest.TestCase):
    def test_empty(self):
        self.assertEqual(stats.percentile([], 0.95), 0.0)

    def test_median(self):
        self.assertEqual(stats.percentile([1, 2, 3, 4], 0.5), 2.5)

    def test_bounds(self):
        self.assertEqual(stats.percentile([1, 2, 3, 4], 0.0), 1)
        self.assertEqual(stats.percentile([1, 2, 3, 4], 1.0), 4)


if __name__ == "__main__":
    unittest.main()
