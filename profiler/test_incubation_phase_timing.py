import unittest

from incubation_phase_timing import summarize


class IncubationPhaseTimingTest(unittest.TestCase):
    def test_joins_by_owner_and_serial(self):
        census = {'overflow': 0, 'rows': [[3_000_000, 4_000_000, 'left', 71, 8, 1, 0]]}
        lives = [dict(owner='left', serial=1, index=8, startNs=1_000_000,
                      readyNs=10_000_000),
                 dict(owner='right', serial=1, index=8, startNs=1_000_000)]
        report = summarize(census, lives, 0)['duringInput']
        self.assertEqual(report['preInitMs']['p50'], 2)
        self.assertEqual(report['initializationMs']['p50'], 1)
        self.assertEqual(report['postInitMs']['p50'], 6)

    def test_rejects_ready_without_initialization(self):
        lives = [dict(owner='left', serial=1, index=8, startNs=1, readyNs=10)]
        with self.assertRaisesRegex(ValueError, 'lacks initialization'):
            summarize({'overflow': 0, 'rows': []}, lives, 0)
