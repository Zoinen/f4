import unittest

from pre_input_attribution import summarize


class PreInputAttributionTests(unittest.TestCase):
    def test_keeps_loading_burst_that_input_only_analysis_omits(self):
        rows = [[1000000, 44000000, 'a', 34],
                [50000000, 51000000, 'a', 34]]
        result = summarize(rows, 0, 50000000, [])
        self.assertEqual(result['largestContainedCommitMs'], 43)
        self.assertEqual(result['delegateCommitElapsedUnionMs'], 43)
        self.assertEqual(result['fullyContainedDelegateCommits'], 1)

    def test_clips_straddling_ranges_without_double_counting_children(self):
        rows = [[0, 3000000, 'a', 34], [1200000, 1500000, 'b', 34]]
        result = summarize(rows, 1000000, 2000000, [])
        self.assertEqual(result['delegateCommitElapsedUnionMs'], 1)
        self.assertEqual(result['straddlingDelegateCommits'], 1)
        self.assertEqual(result['largestContainedCommitMs'], .3)

    def test_ignores_pre_navigation_submissions_and_keeps_drops(self):
        rows = [
            {'QPCTime': '.009', 'Dropped': '0', 'msUntilDisplayed': '20'},
            {'QPCTime': '.011', 'Dropped': '1', 'msUntilDisplayed': '0'},
            {'QPCTime': '.012', 'Dropped': '0', 'msUntilDisplayed': '2'},
        ]
        result = summarize([], 10000000, 20000000, rows)
        self.assertEqual(result['preInputSubmittedPresents'], 2)
        self.assertEqual(result['preInputSubmittedDroppedPresents'], 1)
        self.assertEqual(result['firstPostNavigationSubmissionDisplayedMs'], 4)
        self.assertFalse(result['firstVisibleContentProven'])

    def test_reversed_window_is_not_silently_empty(self):
        with self.assertRaises(ValueError):
            summarize([], 2, 1, [])

    def test_malformed_commit_is_not_silently_dropped(self):
        with self.assertRaises(ValueError):
            summarize([[3, 2, 'a', 34]], 1, 4, [])


if __name__ == '__main__':
    unittest.main()
