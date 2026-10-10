#!/usr/bin/env python3
"""Print portable launcher linker metadata from the CI environment."""

from __future__ import annotations

import datetime
import os
import shlex
import subprocess


APP_PACKAGE = "github.com/unxed/f4/internal/app"


def linker_flags(env: dict[str, str]) -> str:
    version = env.get("F4_BUILD_VERSION", "")
    if env.get("GITHUB_REF", "").startswith("refs/tags/v"):
        version = env["GITHUB_REF_NAME"]
    elif env.get("F4_RELEASE_TAG"):
        version = env["F4_RELEASE_TAG"]
    revision = env.get("F4_BUILD_REVISION") or env.get("GITHUB_SHA", "")[:9]
    if not revision:
        revision = subprocess.check_output(
            ["git", "rev-parse", "--short=9", "HEAD"], text=True
        ).strip()
    build_time = env.get("F4_BUILD_TIME") or datetime.datetime.now(
        datetime.timezone.utc
    ).strftime("%Y-%m-%dT%H:%M:%SZ")
    fields = {
        "buildRevision": revision,
        "buildTime": build_time,
        "buildModified": env.get("F4_BUILD_MODIFIED", "false"),
    }
    flags = ["-s", "-w"]
    for name, value in fields.items():
        flags.extend(["-X", f"{APP_PACKAGE}.{name}={value}"])
    if version:
        symbol = env.get("VERSION_SYMBOL", APP_PACKAGE + ".buildVersion")
        if symbol != APP_PACKAGE + ".buildVersion":
            raise SystemExit(f"unexpected version symbol: {symbol}")
        flags.extend(["-X", f"{symbol}={version}"])
    return shlex.join(flags)


if __name__ == "__main__":
    print(linker_flags(dict(os.environ)))
