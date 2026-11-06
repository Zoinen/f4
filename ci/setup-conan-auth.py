#!/usr/bin/env python3
"""Configure Conan 2 to use the project's Artifactory repositories.

The script is intended for a developer machine.  It asks only for the
already-existing Artifactory password, configures the read and upload remotes,
and authenticates both of them.  The password is entered through getpass and
is supplied to Conan through stdin, so it is not put in shell history or in a
process argument list.
"""

from __future__ import annotations

import argparse
import getpass
import os
import re
import shutil
import subprocess
import sys
from dataclasses import dataclass
from urllib.parse import urlsplit


DEFAULT_ARTIFACTORY_URL = "https://v2202510307500392562.nicesrv.de:9443"
DEFAULT_USERNAME = "admin"
DEFAULT_READ_REMOTE = "f4-conan"
DEFAULT_UPLOAD_REMOTE = "f4-conan-upload"
CONAN_QT_CHECK = "qt/6.11.1:*"


@dataclass(frozen=True)
class ConanSetup:
    """Resolved settings used by the setup operation."""

    artifactory_url: str
    username: str
    read_remote: str
    upload_remote: str

    @property
    def artifactory_root(self) -> str:
        if self.artifactory_url.endswith("/artifactory"):
            return self.artifactory_url
        return f"{self.artifactory_url}/artifactory"

    @property
    def read_url(self) -> str:
        return f"{self.artifactory_root}/api/conan/f4-conan-virtual"

    @property
    def upload_url(self) -> str:
        return f"{self.artifactory_root}/api/conan/f4-conan-local"


def normalize_artifactory_url(value: str) -> str:
    """Validate and normalize the Artifactory base URL."""

    url = value.strip().rstrip("/")
    parsed = urlsplit(url)
    if parsed.scheme not in {"http", "https"} or not parsed.netloc:
        raise ValueError("Artifactory URL must be an absolute http(s) URL")
    if parsed.query or parsed.fragment:
        raise ValueError("Artifactory URL must not contain a query or fragment")
    return url


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Configure Conan 2 authentication for the F4 Artifactory server."
    )
    parser.add_argument(
        "--url",
        default=os.environ.get("F4_ARTIFACTORY_URL", DEFAULT_ARTIFACTORY_URL),
        help="Artifactory base URL (default: the project's server)",
    )
    parser.add_argument(
        "--username",
        default=os.environ.get("F4_ARTIFACTORY_USERNAME", DEFAULT_USERNAME),
        help="Artifactory username (default: admin)",
    )
    parser.add_argument(
        "--read-remote",
        default=DEFAULT_READ_REMOTE,
        help=f"Conan read remote name (default: {DEFAULT_READ_REMOTE})",
    )
    parser.add_argument(
        "--upload-remote",
        default=DEFAULT_UPLOAD_REMOTE,
        help=f"Conan upload remote name (default: {DEFAULT_UPLOAD_REMOTE})",
    )
    parser.add_argument(
        "--skip-check",
        action="store_true",
        help="Do not query the known Qt recipe after login",
    )
    return parser.parse_args(argv)


def resolve_setup(args: argparse.Namespace) -> ConanSetup:
    url = normalize_artifactory_url(args.url)
    username = args.username.strip()
    if not username:
        raise ValueError("Artifactory username cannot be empty")
    if not args.read_remote or not args.upload_remote:
        raise ValueError("Conan remote names cannot be empty")
    if args.read_remote == args.upload_remote:
        raise ValueError("read and upload remote names must be different")
    return ConanSetup(
        artifactory_url=url,
        username=username,
        read_remote=args.read_remote,
        upload_remote=args.upload_remote,
    )


def find_conan() -> str:
    conan = shutil.which("conan")
    if conan is None:
        raise RuntimeError(
            "Conan was not found in PATH. Install Conan 2, reopen the terminal, "
            "and run this script again."
        )

    result = subprocess.run(
        [conan, "--version"],
        check=True,
        capture_output=True,
        text=True,
    )
    version = ((result.stdout or "") + (result.stderr or "")).strip()
    if re.search(r"\bConan version 2(?:\.|$)", version) is None:
        raise RuntimeError(f"Conan 2 is required; detected: {version or 'unknown version'}")
    return conan


def run_conan(conan: str, arguments: list[str], password: str | None = None) -> None:
    """Run Conan, optionally answering its hidden password prompt via stdin."""

    command = [conan, *arguments]
    input_text = None if password is None else f"{password}\n"
    subprocess.run(command, check=True, input=input_text, text=True)


def configure_remotes(conan: str, setup: ConanSetup, password: str) -> None:
    run_conan(
        conan,
        ["remote", "add", setup.read_remote, setup.read_url, "--force"],
    )
    run_conan(
        conan,
        ["remote", "add", setup.upload_remote, setup.upload_url, "--force"],
    )

    # --password without a value makes Conan request it interactively.  Feeding
    # that prompt through the child stdin keeps the secret out of argv on every
    # supported OS while still allowing this script to automate both logins.
    for remote in (setup.read_remote, setup.upload_remote):
        print(f"Logging Conan into {remote}...")
        run_conan(
            conan,
            ["remote", "login", remote, setup.username, "--password"],
            password=password,
        )


def check_read_access(conan: str, setup: ConanSetup) -> None:
    print(f"Checking read access through {setup.read_remote}...")
    run_conan(
        conan,
        ["list", CONAN_QT_CHECK, "--remote", setup.read_remote, "--format", "compact"],
    )


def main(argv: list[str] | None = None) -> int:
    try:
        args = parse_args(argv)
        setup = resolve_setup(args)
        conan = find_conan()
    except (RuntimeError, ValueError, subprocess.CalledProcessError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1

    print(f"Artifactory: {setup.artifactory_url}")
    print(f"Conan read remote: {setup.read_remote} -> {setup.read_url}")
    print(f"Conan upload remote: {setup.upload_remote} -> {setup.upload_url}")
    password = getpass.getpass("Artifactory password: ")
    if not password:
        print("error: password cannot be empty", file=sys.stderr)
        return 1

    try:
        configure_remotes(conan, setup, password)
        if not args.skip_check:
            check_read_access(conan, setup)
    except subprocess.CalledProcessError as error:
        print(
            f"error: Conan command failed with exit code {error.returncode}",
            file=sys.stderr,
        )
        return error.returncode or 1
    finally:
        # This does not guarantee immediate memory erasure in Python, but it
        # shortens the lifetime of the reference after the child processes exit.
        password = ""

    print("Done. Conan remotes are configured and authenticated.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
