import unittest
from requested_row_timeline import summarize


def event(t, kind, serial=1, detail=0, index=7):
    return [t * 1000000, t * 1000000 + 10, 'abc', kind, index, serial, detail]


class RequestedRowTimelineTests(unittest.TestCase):
    def test_successful_lifetime_separates_waits(self):
        result = summarize([event(0,64), event(10,65), event(30,66),
                            event(35,67), event(36,70)], 20_000_000, -1)
        self.assertEqual(result['timings']['queueWaitMs']['max'], 10)
        self.assertEqual(result['timings']['constructionElapsedMs']['max'], 20)
        self.assertEqual(result['timings']['readyToTakeMs']['max'], 5)
        self.assertEqual(result['timings']['takeToPublicationMs']['max'], 1)
        self.assertEqual(result['maximumConstructingOrUnclaimedReadyByOwner'], {'abc': 1})
        self.assertEqual(result['unfinishedRequests'], [])

    def test_ready_discard_and_queued_replacement_are_distinct(self):
        result = summarize([event(0,64), event(1,65), event(2,66), event(3,68,detail=4),
                            event(4,64,2), event(5,68,2,1)], 10_000_000)
        self.assertEqual(result['canceledByReason'], {'pool-satisfied': 1, 'identity-replaced': 1})
        self.assertEqual(result['canceledReadyByReason'], {'pool-satisfied': 1})
        self.assertEqual(result['stillConstructedByOwner'], {'abc': 0})

    def test_orphans_and_unfinished_requests_are_retained(self):
        result = summarize([event(0,65), event(1,64,2)], 0)
        self.assertEqual(len(result['orphanEvents']), 1)
        self.assertEqual(len(result['unfinishedRequests']), 1)

    def test_duplicate_serial_is_rejected(self):
        with self.assertRaises(ValueError):
            summarize([event(0,64), event(1,64)], 0)

    def test_publication_before_claim_is_rejected(self):
        with self.assertRaises(ValueError):
            summarize([event(0,64), event(1,65), event(2,66), event(3,70)], 0)

    def test_failed_construction_releases_capacity(self):
        result = summarize([event(0,64),event(1,65),event(2,66,detail=1)], 0)
        self.assertEqual(result['stillConstructedByOwner'], {'abc': 0})
        self.assertEqual(len(result['unfinishedRequests']), 1)


if __name__ == '__main__':
    unittest.main()
