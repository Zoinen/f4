import unittest

from capture_foreground import verify


class CaptureForegroundTests(unittest.TestCase):
    def setUp(self):
        self.packet = {'wheel': [{'active': True, 'nativeForeground': True}]}
        self.navigation = {'active': True, 'nativeForeground': True}
        self.admission = {'foregroundPid': 42, 'observer': {
            'cleanDesktopObserverCheck': True, 'observerProcesses': []}}
        self.identity = {'qtPid': 42}

    def verdict(self):
        return verify(self.packet, self.navigation, self.admission, self.identity)

    def test_accepts_independent_native_evidence_without_claiming_motion(self):
        result = self.verdict()
        self.assertTrue(result['acceptedNativeForegroundScope'])
        self.assertFalse(result['physicalMotionVerified'])
        self.assertFalse(result['fullPerformanceAccepted'])

    def test_qt_active_does_not_override_native_foreground_loss(self):
        self.packet['wheel'][0]['nativeForeground'] = False
        result = self.verdict()
        self.assertFalse(result['acceptedNativeForegroundScope'])
        self.assertEqual(result['inputSamplesWithoutNativeForeground'], 1)

    def test_old_capture_without_native_fields_is_unproven(self):
        del self.packet['wheel'][0]['nativeForeground']
        self.assertFalse(self.verdict()['acceptedNativeForegroundScope'])

    def test_empty_input_is_not_vacuously_accepted(self):
        self.packet['wheel'] = []
        self.assertFalse(self.verdict()['acceptedNativeForegroundScope'])

    def test_wrong_process_or_missing_identity_is_rejected(self):
        self.admission['foregroundPid'] = 43
        self.assertFalse(self.verdict()['acceptedNativeForegroundScope'])
        self.admission.pop('foregroundPid')
        self.identity.pop('qtPid')
        self.assertFalse(self.verdict()['acceptedNativeForegroundScope'])

    def test_observer_flag_alone_cannot_hide_observer_process(self):
        self.admission['observer']['observerProcesses'] = [{'pid': 99}]
        self.assertFalse(self.verdict()['acceptedNativeForegroundScope'])

    def test_navigation_must_also_be_foreground(self):
        self.navigation['nativeForeground'] = False
        self.assertFalse(self.verdict()['acceptedNativeForegroundScope'])


if __name__ == '__main__':
    unittest.main()
