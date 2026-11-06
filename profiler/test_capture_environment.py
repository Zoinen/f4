import subprocess
import unittest
from unittest.mock import patch

import capture_environment as guard


class CaptureEnvironmentTests(unittest.TestCase):
    def test_real_helper_is_detected(self):
        output = '"codex-computer-use.exe","21496","Console","1","42,000 K"\n'
        self.assertEqual(guard.parse_observers(output), [
            {"image": "codex-computer-use.exe", "pid": 21496}])

    def test_multiple_clients_remain_visible(self):
        output = '"codex-computer-use.exe","12"\n"CODEX-COMPUTER-USE.EXE","34"\n'
        self.assertEqual([x["pid"] for x in guard.parse_observers(output)], [12, 34])

    def test_unrelated_app_is_not_an_observer(self):
        self.assertEqual(guard.parse_observers('"codex-computer-use.exe.backup","12"\n'), [])

    def test_no_match_message_is_not_a_process(self):
        self.assertEqual(guard.parse_observers("INFO: No tasks match the specified criteria.\n"), [])

    def test_invalid_matching_pid_fails(self):
        with self.assertRaises(ValueError):
            guard.parse_observers('"codex-computer-use.exe","unknown"\n')

    def test_active_helper_rejects_clean_measurement(self):
        with self.assertRaisesRegex(RuntimeError, "supported reset"):
            guard.require_clean({"supported": True, "cleanDesktopObserverCheck": False})

    def test_explicit_attribution_control_does_not_claim_clean(self):
        state = {"supported": True, "cleanDesktopObserverCheck": False}
        guard.require_clean(state, allow_observer=True)
        self.assertFalse(state["cleanDesktopObserverCheck"])

    def test_query_failure_is_not_clean(self):
        with patch.object(guard.os, "name", "nt"), patch.object(
                guard.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "tasklist")):
            with self.assertRaises(subprocess.CalledProcessError):
                guard.inspect_environment()

    def test_unsupported_platform_does_not_pass(self):
        with self.assertRaises(RuntimeError):
            guard.require_clean({"supported": False, "reason": "not measured"})


if __name__ == "__main__":
    unittest.main()
