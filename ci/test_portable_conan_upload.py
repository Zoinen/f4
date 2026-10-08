"""Tests for the Linux Conan publication gate."""

from __future__ import annotations

import importlib.util
import pathlib
import unittest
from unittest.mock import Mock, patch

SCRIPT = pathlib.Path(__file__).with_name("upload-conan-packages.py")
SPEC = importlib.util.spec_from_file_location("upload_conan_packages", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
uploader = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(uploader)


def package_index(
    catalog_name: str,
    recipe_revision: str = "recipe-revision",
    package_revision: str = "package-revision",
) -> dict:
    return {
        catalog_name: {
            "qt/6.11.1": {
                "revisions": {
                    recipe_revision: {
                        "packages": {
                            "x86-package": {
                                "revisions": {package_revision: {}},
                                "info": {
                                    "settings": {"os": "Linux", "arch": "x86_64"}
                                }
                            },
                            "arm-package": {
                                "revisions": {package_revision: {}},
                                "info": {
                                    "settings": {"os": "Linux", "arch": "armv8"}
                                }
                            },
                        }
                    }
                }
            }
        }
    }


class ConanPublicationGateTests(unittest.TestCase):
    @patch.object(uploader.subprocess, "run")
    def test_local_cache_listing_requests_package_revisions(self, run) -> None:
        run.return_value = Mock(stdout="{}")

        uploader.list_recipe_packages()

        run.assert_called_once_with(
            ["conan", "list", "*/*:*#*", "--format=json"],
            check=True,
            capture_output=True,
            text=True,
        )

    @patch.object(uploader.subprocess, "run")
    def test_remote_listing_requests_all_recipe_and_package_revisions(
        self, run
    ) -> None:
        run.return_value = Mock(stdout="{}")

        uploader.list_all_recipe_package_revisions(
            "qt/6.11.1", "f4-conan-upload"
        )

        run.assert_called_once_with(
            [
                "conan",
                "list",
                "qt/6.11.1#*:*#*",
                "--remote",
                "f4-conan-upload",
                "--format=json",
            ],
            check=True,
            capture_output=True,
            text=True,
        )

    def test_local_package_architectures_are_read_from_conan_json(self) -> None:
        data = package_index("Local Cache")

        self.assertEqual(
            uploader.package_revisions_for_arch(data, "qt/6.11.1", "armv8"),
            {("recipe-revision", "arm-package", "package-revision")},
        )

    def test_remote_package_architecture_is_verified(self) -> None:
        local = package_index("Local Cache")
        remote = package_index("f4-conan-upload")
        local_packages = uploader.verify_required_packages(
            local,
            ["qt/6.11.1"],
            "armv8",
            "test local cache",
        )

        uploader.verify_uploaded_packages(
            local_packages,
            remote,
            "armv8",
            "test remote",
            "f4-conan-upload",
        )

    def test_remote_package_revision_does_not_require_local_settings_metadata(
        self,
    ) -> None:
        local = package_index("Local Cache")
        remote = package_index("f4-conan-upload")
        del remote["f4-conan-upload"]["qt/6.11.1"]["revisions"][
            "recipe-revision"
        ]["packages"]["arm-package"]["info"]
        local_packages = uploader.verify_required_packages(
            local,
            ["qt/6.11.1"],
            "armv8",
            "test local cache",
        )

        uploader.verify_uploaded_packages(
            local_packages,
            remote,
            "armv8",
            "test remote",
            "f4-conan-upload",
        )

    def test_missing_architecture_fails_closed(self) -> None:
        data = package_index("Local Cache")

        with self.assertRaisesRegex(SystemExit, "missing armv7 packages"):
            uploader.verify_required_packages(
                data,
                ["qt/6.11.1"],
                "armv7",
                "test local cache",
            )

    def test_old_architecture_package_does_not_count_as_uploaded(self) -> None:
        local = package_index("Local Cache")
        old_remote = package_index("f4-conan-upload", "video-only-recipe")
        local_packages = uploader.verify_required_packages(
            local,
            ["qt/6.11.1"],
            "armv8",
            "test local cache",
        )

        with self.assertRaisesRegex(SystemExit, "cannot read the uploaded packages"):
            uploader.verify_uploaded_packages(
                local_packages,
                old_remote,
                "armv8",
                "test remote",
                "f4-conan-upload",
            )

    def test_old_package_revision_does_not_count_as_rebuilt_binary(self) -> None:
        local = package_index("Local Cache")
        old_remote = package_index(
            "f4-conan-upload",
            package_revision="old-package-revision",
        )
        local_packages = uploader.verify_required_packages(
            local,
            ["qt/6.11.1"],
            "armv8",
            "test local cache",
        )

        with self.assertRaisesRegex(
            SystemExit,
            "qt/6.11.1#recipe-revision:arm-package#package-revision",
        ):
            uploader.verify_uploaded_packages(
                local_packages,
                old_remote,
                "armv8",
                "test remote",
                "f4-conan-upload",
            )


if __name__ == "__main__":
    unittest.main()
