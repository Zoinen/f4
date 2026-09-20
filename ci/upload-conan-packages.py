#!/usr/bin/env python3
"""Publish the completed Conan cache from a CI job to Artifactory."""

from __future__ import annotations

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
    subprocess.run(
        [
            "conan",
            "upload",
            "*",
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
