import unittest
from entry_callback_timeline import covered_ns


class IntervalCoverageTests(unittest.TestCase):
    def test_nested_spans_are_not_added_twice(self):
        self.assertEqual(covered_ns([(2, 8), (3, 4), (6, 9)], 0, 10), 7)

    def test_boundary_crossing_is_clipped(self):
        self.assertEqual(covered_ns([(-20, 3), (8, 30)], 0, 10), 5)

    def test_disjoint_and_touching_spans(self):
        self.assertEqual(covered_ns([(8, 9), (2, 4), (4, 6)], 0, 10), 5)

    def test_zero_length_and_outside_spans(self):
        self.assertEqual(covered_ns([(0, 0), (-3, 0), (10, 20)], 0, 10), 0)

    def test_reversed_window_is_rejected(self):
        with self.assertRaises(ValueError):
            covered_ns([], 10, 0)


if __name__ == '__main__':
    unittest.main()
