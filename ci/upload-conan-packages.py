#!/usr/bin/env python3
"""Publish the completed Conan cache from a CI job to Artifactory."""

from __future__ import annotations

import json
import os
import subprocess
import sys


def main() -> int:
    remote_url = os.environ.get("F4_CONAN_UPLOAD_URL", "")
    token = os.environ.get("F4_CONAN_UPLOAD_TOKEN", "")
    if not remote_url or not token:
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
    list_result = subprocess.run(
        ["conan", "list", "*/*:*", "--format=json"],
        check=True,
        capture_output=True,
        text=True,
    )
    local_cache = json.loads(list_result.stdout).get("Local Cache", {})
    recipe_refs = sorted(local_cache)
    if sys.platform == "win32" or os.environ.get("RUNNER_OS") == "Windows":
        recipe_refs = [ref for ref in recipe_refs if not ref.startswith("msys2/")]

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
    print("Conan package graph uploaded")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except subprocess.CalledProcessError as error:
        print(f"Conan package upload failed with exit code {error.returncode}", file=sys.stderr)
        raise
