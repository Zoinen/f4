#!/usr/bin/env python3
"""Publish the completed Conan cache from a CI job to Artifactory."""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
from datetime import datetime, timezone


def package_revisions(
    data: dict,
    recipe_ref: str,
    expected_arch: str | None = None,
    remote_name: str = "",
    latest_only: bool = False,
) -> set:
    """Return exact recipe/package revisions, optionally selecting latest revisions."""
    catalog_name = remote_name or "Local Cache"
    catalog = data.get(catalog_name, {})
    recipe = catalog.get(recipe_ref, {})
    package_revisions = set()
    recipe_revisions = recipe.get("revisions", {})
    if latest_only and recipe_revisions:
        recipe_revision = max(
            recipe_revisions,
            key=lambda key: _revision_timestamp(recipe_revisions[key]),
        )
        recipe_revisions = {recipe_revision: recipe_revisions[recipe_revision]}
    for recipe_revision, revision in recipe_revisions.items():
        for package_id, package in revision.get("packages", {}).items():
            settings = package.get("info", {}).get("settings", {})
            if (
                expected_arch is not None
                and str(settings.get("arch", "")) != expected_arch
            ):
                continue
            package_revision_data = package.get("revisions", {})
            if latest_only and package_revision_data:
                package_revision = max(
                    package_revision_data,
                    key=lambda key: _revision_timestamp(
                        package_revision_data[key]
                    ),
                )
                package_revision_data = {
                    package_revision: package_revision_data[package_revision]
                }
            for package_revision in package_revision_data:
                package_revisions.add(
                    (recipe_revision, package_id, package_revision)
                )
    return package_revisions


def _revision_timestamp(revision: dict) -> float:
    """Convert Conan's list-JSON revision timestamp to a sortable number."""
    timestamp = revision.get("timestamp", "")
    if isinstance(timestamp, (int, float)):
        return float(timestamp)
    if not isinstance(timestamp, str):
        return 0.0
    for time_format in ("%Y-%m-%d %H:%M:%S UTC", "%Y-%m-%d %H:%M:%S"):
        try:
            return datetime.strptime(timestamp, time_format).replace(
                tzinfo=timezone.utc
            ).timestamp()
        except ValueError:
            pass
    try:
        return datetime.fromisoformat(timestamp.replace("Z", "+00:00")).timestamp()
    except ValueError:
        return 0.0


def package_revisions_for_arch(
    data: dict,
    recipe_ref: str,
    expected_arch: str,
    remote_name: str = "",
    latest_only: bool = False,
) -> set:
    """Return exact recipe/package revisions for binaries of one architecture."""
    return package_revisions(
        data, recipe_ref, expected_arch, remote_name, latest_only=latest_only
    )


def verify_required_packages(
    data: dict,
    recipe_refs: list,
    expected_arch: str,
    source: str,
    remote_name: str = "",
) -> dict:
    required_packages = {}
    missing = []
    for recipe_ref in recipe_refs:
        package_revisions = package_revisions_for_arch(
            data, recipe_ref, expected_arch, remote_name, latest_only=True
        )
        if not package_revisions:
            missing.append(recipe_ref)
        else:
            required_packages[recipe_ref] = package_revisions
    if missing:
        raise SystemExit(
            f"{source} is missing {expected_arch} packages for: {', '.join(missing)}"
        )
    return required_packages


def verify_uploaded_packages(
    required_packages: dict,
    data: dict,
    expected_arch: str,
    source: str,
    remote_name: str,
) -> None:
    missing = []
    for recipe_ref, expected in required_packages.items():
        # The local cache supplies the architecture-filtered package IDs. Conan
        # remotes may list revisions without the local `info.settings` metadata,
        # so remote verification must compare the exact references only.
        available = package_revisions(data, recipe_ref, remote_name=remote_name)
        for recipe_revision, package_id, package_revision in sorted(
            expected - available
        ):
            missing.append(
                f"{recipe_ref}#{recipe_revision}:{package_id}#{package_revision}"
            )
    if missing:
        raise SystemExit(
            f"{source} cannot read the uploaded packages: {', '.join(missing)}"
        )


def list_recipe_packages(recipe_ref: str = "*/*:*#*", remote_name: str = "") -> dict:
    command = ["conan", "list", recipe_ref]
    if remote_name:
        command.extend(["--remote", remote_name])
    command.extend(["--format=json"])
    result = subprocess.run(command, check=True, capture_output=True, text=True)
    return json.loads(result.stdout)


def list_all_recipe_package_revisions(recipe_ref: str, remote_name: str) -> dict:
    """List every recipe and package revision for a reference on a remote."""
    return list_recipe_packages(f"{recipe_ref}#*:*#*", remote_name)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--required",
        action="store_true",
        help="fail instead of skipping when Artifactory credentials are absent",
    )
    parser.add_argument(
        "--verify-required-packages",
        action="store_true",
        help="verify required local packages and their Artifactory publication",
    )
    args = parser.parse_args()

    remote_url = os.environ.get("F4_CONAN_UPLOAD_URL", "")
    token = os.environ.get("F4_CONAN_UPLOAD_TOKEN", "")
    if not remote_url or not token:
        if args.required:
            print(
                "error: required Conan upload URL/token is not configured",
                file=sys.stderr,
            )
            return 2
        print("Conan upload remote is not configured; skipping package upload")
        return 0

    remote_name = os.environ.get("F4_CONAN_UPLOAD_REMOTE_NAME", "f4-conan-upload")
    username = os.environ.get("F4_CONAN_USERNAME", "admin")

    subprocess.run(
        ["conan", "remote", "add", remote_name, remote_url, "--index", "0", "--force"],
        check=True,
    )
    subprocess.run(
        ["conan", "remote", "login", remote_name, username, "--password", token],
        check=True,
    )

    # Each job has its own CONAN_HOME and has just built the graph needed by
    # that platform.  Uploading the complete local cache preserves all
    # transitive binaries (including native Qt build tools) and is safe to
    # repeat: Conan skips artifacts that already exist at the same revision.
    # Conan's MSYS2 package contains a dangling/special `etc/mtab` entry on
    # GitHub's Windows runners.  `conan upload "*" --check` tries to hash that
    # entry and aborts before it can publish any of the useful target graph.
    # Enumerate recipe references first and omit only this build-tool package
    # on Windows; the virtual remote still provides it from ConanCenter.
    local_data = list_recipe_packages()
    local_cache = local_data.get("Local Cache", {})
    recipe_refs = sorted(local_cache)
    if sys.platform == "win32" or os.environ.get("RUNNER_OS") == "Windows":
        recipe_refs = [ref for ref in recipe_refs if not ref.startswith("msys2/")]

    required_refs = [
        recipe_ref.strip()
        for recipe_ref in os.environ.get("F4_CONAN_REQUIRED_RECIPE_REFS", "").split(",")
        if recipe_ref.strip()
    ]
    expected_arch = os.environ.get("F4_CONAN_EXPECTED_ARCH", "").strip()
    if args.verify_required_packages:
        if not required_refs or not expected_arch:
            raise SystemExit(
                "package verification requires F4_CONAN_REQUIRED_RECIPE_REFS "
                "and F4_CONAN_EXPECTED_ARCH"
            )
        required_packages = verify_required_packages(
            local_data,
            required_refs,
            expected_arch,
            "local Conan cache",
        )
    else:
        required_packages = {}

    for recipe_ref in recipe_refs:
        subprocess.run(
            [
                "conan",
                "upload",
                f"{recipe_ref}:*",
                "--remote",
                remote_name,
                "--confirm",
                "--check",
            ],
            check=True,
        )

    if args.verify_required_packages:
        read_remote_name = os.environ.get("F4_CONAN_REMOTE_NAME", "f4-conan")
        for verify_remote in dict.fromkeys((remote_name, read_remote_name)):
            for recipe_ref in required_refs:
                remote_data = list_all_recipe_package_revisions(
                    recipe_ref, verify_remote
                )
                verify_uploaded_packages(
                    {recipe_ref: required_packages[recipe_ref]},
                    remote_data,
                    expected_arch,
                    f"Artifactory remote {verify_remote}",
                    verify_remote,
                )

    print("Conan package graph uploaded")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as error:
        print(f"Conan package upload failed with exit code {error.returncode}", file=sys.stderr)
        raise
