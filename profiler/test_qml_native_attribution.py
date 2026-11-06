import unittest

from qml_native_attribution import clipped_union_ns, contained_commits, summarize_target


class NativeAttributionTests(unittest.TestCase):
    def test_clips_and_unions_nested_ranges(self):
        self.assertEqual(clipped_union_ns(
            [(0, 40), (5, 10), (30, 60), (80, 110)], 10, 100), 70)

    def test_rejects_other_owners_and_straddling_commits(self):
        target = [100, 200, 'a', 38]
        matching = [110, 190, 'a', 34]
        rows = [matching, [110, 190, 'b', 34], [90, 190, 'a', 34],
                [110, 210, 'a', 34]]
        self.assertEqual(contained_commits(rows, target), [matching])

    def test_phase_duration_uses_payload_not_marker_span(self):
        target = [1000000, 10000000, 'a', 38]
        rows = [[1100000, 9900000, 'a', 34],
                [9800000, 9800100, 'a', 50, 8000000],
                [9800200, 9800300, 'b', 50, 9000000]]
        ranges = [{'start': 1200000, 'end': 9000000, 'type': 'Creating'},
                  {'start': 1300000, 'end': 8000000, 'type': 'Creating'}]
        summary = summarize_target(rows, ranges, target, 0)
        self.assertEqual(summary['phaseMs']['prepare'], 8)
        self.assertEqual(summary['qmlCreationElapsedUnionMs'], 7.8)
        self.assertEqual(summary['containedCommitCount'], 1)

    def test_suspended_creation_is_not_charged_as_inner_construction(self):
        target = [1000000, 2000000, 'a', 38]
        ranges = [{'start': 0, 'end': 3000000, 'type': 'Creating'},
                  {'start': 1200000, 'end': 1500000, 'type': 'Creating'}]
        summary = summarize_target([], ranges, target, 0)
        self.assertEqual(summary['qmlCreationElapsedUnionMs'], 1)
        self.assertEqual(summary['containedQmlCreationUnionMs'], .3)


if __name__ == '__main__':
    unittest.main()
