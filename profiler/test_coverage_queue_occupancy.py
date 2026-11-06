import copy
import unittest

from coverage_queue_occupancy import analyze


class CoverageQueueOccupancyTests(unittest.TestCase):
    def setUp(self):
        self.probe = dict(overflow=0, wrongWriter=0,
                          frames=[dict(endNs=100, layout='a')])
        self.coverage = dict(holds=[dict(holdNs=120, syncNs=100, blockers=[
            dict(index=7, serial=1, stage='queued')])])
        self.lives = [dict(owner='a', index=7, serial=1, requestNs=10, startNs=110),
                      dict(owner='a', index=8, serial=2, requestNs=5, startNs=20,
                           readyNs=90, takeNs=105)]

    def run_join(self):
        return analyze(self.probe, self.coverage, self.lives, 1)

    def test_ready_unclaimed_root_occupies_slot_and_two_clocks_differ(self):
        row = self.run_join()['rows'][0]
        self.assertEqual(row['capacityAtSync'], 'full')
        self.assertEqual(row['occupantsAtSync'][0]['stage'], 'ready')
        self.assertEqual(row['occupantsAtHeldRender'][0]['index'], 7)
        self.assertEqual(row['stageAtHeldRender'], 'constructing')

    def test_taken_canceled_and_failed_roots_release_slots(self):
        self.lives[1]['takeNs'] = 99
        self.assertEqual(self.run_join()['capacityAtSync'], {'spare': 1})
        self.lives[1].pop('takeNs')
        self.lives[1]['cancelNs'] = 99
        self.assertEqual(self.run_join()['capacityAtSync'], {'spare': 1})
        self.lives[1].pop('cancelNs')
        self.lives[1]['failed'] = True
        self.assertEqual(self.run_join()['capacityAtSync'], {'spare': 1})

    def test_other_owners_do_not_fill_slot_or_collide_serials(self):
        self.lives[1]['owner'] = 'b'
        self.lives[1]['serial'] = 1
        self.assertEqual(self.run_join()['capacityAtSync'], {'spare': 1})

    def test_missing_and_ambiguous_syncs_retain_unmatched_hold(self):
        self.probe['frames'] = []
        self.assertEqual(len(self.run_join()['unmatchedHolds']), 1)
        self.probe['frames'] = [dict(endNs=100, layout='a'), dict(endNs=100, layout='b')]
        self.assertEqual(len(self.run_join()['unmatchedHolds']), 1)
        self.coverage['holds'][0]['unmatched'] = 'outside-probe-window'
        self.assertEqual(self.run_join()['unmatchedHolds'][0]['reason'], 'outside-probe-window')

    def test_invalid_scope_and_lifetimes_fail_closed(self):
        self.probe['overflow'] = 1
        with self.assertRaises(ValueError):
            self.run_join()
        self.probe['overflow'] = 0
        self.lives.append(copy.deepcopy(self.lives[0]))
        with self.assertRaises(ValueError):
            self.run_join()
        self.lives.pop()
        self.lives.append(dict(owner='a', index=9, serial=3, requestNs=1, startNs=2))
        with self.assertRaises(ValueError):
            self.run_join()


if __name__ == '__main__':
    unittest.main()
