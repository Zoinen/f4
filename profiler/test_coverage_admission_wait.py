import unittest

from coverage_admission_wait import summarize


class CoverageAdmissionWaitTest(unittest.TestCase):
    def test_counts_ready_root_until_taken_and_ignores_other_owner(self):
        ms = 1_000_000
        rows = [dict(owner='left', serial=3, syncNs=4*ms),
                dict(owner='left', serial=3, syncNs=3*ms)]
        lifetimes = [
            dict(owner='left', serial=1, startNs=ms, readyNs=2*ms, takeNs=5*ms),
            dict(owner='left', serial=2, startNs=7*ms, takeNs=9*ms),
            dict(owner='left', serial=3, startNs=10*ms),
            dict(owner='right', serial=1, startNs=6*ms),
        ]
        result = summarize(rows, lifetimes, 1)
        self.assertEqual(result['rows'], [
            dict(owner='left', serial=3, started=True, waitMs=7,
                 spareSlotMs=3, otherStarts=1),
        ])

    def test_reports_requests_without_terminal_evidence(self):
        rows = [dict(owner='left', serial=1, syncNs=3)]
        result = summarize(rows, [dict(owner='left', serial=1)], 2)
        self.assertEqual(result['unterminated'], 1)
        self.assertEqual(result['rows'], [])


if __name__ == '__main__':
    unittest.main()
