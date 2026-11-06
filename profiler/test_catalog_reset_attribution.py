import unittest

from catalog_reset_attribution import attribute


class CatalogResetAttributionTests(unittest.TestCase):
    def setUp(self):
        self.scope = [110, 150, 'model', 8, 48, 192, 1]
        self.cancel = [120, 121, 'layout', 68, 9, 3, 5]
        self.life = dict(owner='layout', serial=3, index=9, cancelNs=120)

    def join(self, scopes, lives=None, overflow=0):
        return attribute(dict(overflow=overflow, rows=scopes+[self.cancel]),
                         [self.life] if lives is None else lives, 100)

    def test_queued_and_started_work_are_distinguished(self):
        group = self.join([self.scope])['groups'][0]
        self.assertEqual((group['incomingEntries'], group['previousLogicalCount']), (48, 192))
        self.assertEqual((group['queued'], group['started']), (1, 0))
        self.life['startNs'] = 115
        group = self.join([self.scope])['groups'][0]
        self.assertEqual((group['queued'], group['started']), (0, 1))

    def test_ambiguity_and_owner_mismatch_remain_unmatched(self):
        self.assertEqual(self.join([self.scope, self.scope])['unmatchedReasons'],
                         {'ambiguous-scope': 1})
        self.assertEqual(self.join([self.scope], [])['unmatchedReasons'],
                         {'missing-lifetime': 1})
        self.assertEqual(self.join([])['unmatchedReasons'], {'no-scope': 1})

    def test_incomplete_trace_rejected(self):
        with self.assertRaises(ValueError):
            self.join([self.scope], overflow=1)
