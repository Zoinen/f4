import copy
import unittest
from coverage_row_attribution import attribute


class CoverageRowAttributionTests(unittest.TestCase):
    def setUp(self):
        self.frame = dict(beginNs=90, endNs=100, job='abc', layout='def',
                          firstRow=0, rowCount=1, logicalY=50, viewportTop=50,
                          viewportHeight=100, minimum=0, maximum=100, anchorReady=True)
        self.probe = dict(overflow=0, wrongWriter=0, startedNs=0, captureWindowMs=1,
                          frames=[self.frame],
                          rows=[[0, 7, 0, 1, False, '0', 200, 250, 0, 50,
                                 0, 100, 0, 100, True, True]])
        self.hold = dict(timeNs=110, job=0xabc, requestedY=120,
                         minimum=0, coverageMaximum=100, anchorReady=True)
        self.life = dict(owner='def', index=7, serial=1, requestNs=10,
                         startNs=80, readyNs=105, takeNs=120, publishNs=130)

    def run_join(self, lives=None):
        return attribute(self.probe, [self.hold], lives or [self.life], 0)

    def test_stage_is_evaluated_at_sync_not_later_hold(self):
        result = self.run_join()
        self.assertEqual(result['matchedHolds'], 1)
        self.assertEqual(result['blockerOccurrencesByStage'], {'constructing': 1})
        self.assertEqual(result['blockerOccurrencesAtHeldRender'], {'ready': 1})

    def test_gui_publication_does_not_imply_synchronized_scene(self):
        self.life.update(readyNs=101, takeNs=102, publishNs=103)
        result = self.run_join()
        self.assertEqual(result['blockerOccurrencesByStage'], {'constructing': 1})
        self.assertEqual(result['blockerOccurrencesAtHeldRender'], {'published': 1})

    def test_future_sync_is_not_used(self):
        future = copy.deepcopy(self.frame)
        future.update(beginNs=111, endNs=112, maximum=500)
        self.probe['frames'].append(future)
        self.assertEqual(self.run_join()['matchedHolds'], 1)

    def test_backward_and_anchor_blockers(self):
        self.probe['rows'][0][6:8] = [-40, 0]
        self.hold['requestedY'] = -5
        self.assertEqual(self.run_join()['holds'][0]['direction'], 'backward')
        self.probe['rows'][0][6:8] = [60, 80]
        self.hold['anchorReady'] = self.frame['anchorReady'] = False
        self.assertEqual(self.run_join()['holds'][0]['direction'], 'anchor')
        self.assertEqual(self.run_join()['matchedHolds'], 1)

    def test_canceled_root_and_replacement_are_not_confused(self):
        old = dict(self.life, cancelNs=70)
        old.pop('takeNs')
        replacement = dict(owner='def', index=7, serial=2, requestNs=75)
        result = self.run_join([old, replacement])
        self.assertEqual(result['blockerOccurrencesByStage'], {'queued': 1})
        self.assertEqual(result['holds'][0]['blockers'][0]['serial'], 2)

    def test_ready_failure_and_no_request_are_retained(self):
        self.life['readyNs'] = 95
        self.assertEqual(self.run_join()['blockerOccurrencesByStage'], {'ready': 1})
        self.life['failed'] = True
        self.assertEqual(self.run_join()['blockerOccurrencesByStage'], {'failed': 1})
        self.life['owner'] = 'other'
        self.assertEqual(self.run_join()['blockerOccurrencesByStage'], {'never-requested': 1})

    def test_scope_mismatches_are_retained(self):
        self.hold['coverageMaximum'] += 1
        self.assertEqual(self.run_join()['unmatchedByReason'], {'coverage-state-mismatch': 1})
        self.hold['timeNs'] = 2_000_000
        self.assertEqual(self.run_join()['unmatchedByReason'], {'outside-probe-window': 1})
        self.hold['timeNs'] = 50
        self.assertEqual(self.run_join()['unmatchedByReason'], {'no-prior-sync': 1})

    def test_corrupt_probe_and_overlapping_lifetimes_are_rejected(self):
        with self.assertRaises(ValueError):
            self.run_join([self.life, dict(self.life, serial=2)])
        self.probe['overflow'] = 1
        with self.assertRaises(ValueError):
            self.run_join()
        self.probe['overflow'] = 0
        self.frame['rowCount'] = 2
        with self.assertRaises(ValueError):
            self.run_join()


if __name__ == '__main__':
    unittest.main()
