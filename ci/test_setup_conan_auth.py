from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import sys
import unittest
from unittest import mock


CI_ROOT = pathlib.Path(__file__).parent
SCRIPT = CI_ROOT / "setup-conan-auth.py"
SPEC = importlib.util.spec_from_file_location("setup_conan_auth", SCRIPT)
if SPEC is None or SPEC.loader is None:
    raise AssertionError(f"cannot load {SCRIPT}")
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)


class SetupConanAuthTest(unittest.TestCase):
    def test_remote_urls_use_the_expected_artifactory_repositories(self) -> None:
        setup = MODULE.ConanSetup(
            "https://example.test:9443",
            "admin",
            "f4-conan",
            "f4-conan-upload",
        )

        self.assertEqual(
            setup.read_url,
            "https://example.test:9443/artifactory/api/conan/f4-conan-virtual",
        )
        self.assertEqual(
            setup.upload_url,
            "https://example.test:9443/artifactory/api/conan/f4-conan-local",
        )

    def test_existing_artifactory_path_is_not_duplicated(self) -> None:
        setup = MODULE.ConanSetup(
            "https://example.test/artifactory",
            "admin",
            "read",
            "upload",
        )

        self.assertEqual(
            setup.read_url,
            "https://example.test/artifactory/api/conan/f4-conan-virtual",
        )

    @mock.patch.object(MODULE.getpass, "getpass", return_value="test secret")
    @mock.patch.object(MODULE.shutil, "which", return_value="/usr/bin/conan")
    @mock.patch.object(MODULE.subprocess, "run")
    def test_main_never_puts_password_in_conan_arguments(
        self,
        run: mock.Mock,
        _which: mock.Mock,
        _getpass: mock.Mock,
    ) -> None:
        run.return_value = subprocess.CompletedProcess([], 0, stdout="Conan version 2.29.1")

        result = MODULE.main(["--skip-check"])

        self.assertEqual(result, 0)
        commands = [call.args[0] for call in run.call_args_list]
        login_commands = [command for command in commands if "login" in command]
        self.assertEqual(len(login_commands), 2)
        for command in login_commands:
            self.assertEqual(command[-1], "--password")
            self.assertNotIn("test secret", command)

        login_calls = [call for call in run.call_args_list if "login" in call.args[0]]
        self.assertEqual([call.kwargs["input"] for call in login_calls], ["test secret\n"] * 2)


if __name__ == "__main__":
    unittest.main()
