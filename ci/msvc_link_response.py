#!/usr/bin/env python3
"""Preserve linker response arguments while avoiding MSVC's long LF-line bug."""

from __future__ import annotations

import codecs
from pathlib import Path
import subprocess
import sys
import tempfile


def normalize_newlines(data: bytes) -> bytes:
    for bom, encoding in ((codecs.BOM_UTF16_LE, "utf-16-le"),
                          (codecs.BOM_UTF16_BE, "utf-16-be")):
        if data.startswith(bom):
            text = data[len(bom):].decode(encoding)
            return bom + text.replace("\r\n", "\n").replace("\n", "\r\n").encode(encoding)
    return data.replace(b"\r\n", b"\n").replace(b"\n", b"\r\n")


def run(arguments: list[str]) -> int:
    if not arguments:
        print("usage: msvc_link_response.py linker [arguments ...]", file=sys.stderr)
        return 2
    command = list(arguments)
    temporary: list[Path] = []
    try:
        for index, argument in enumerate(command[1:], 1):
            if not argument.startswith("@"):
                continue
            source = Path(argument[1:])
            if not source.is_file():
                continue  # Preserve the linker's own missing-input diagnostic.
            original = source.read_bytes()
            normalized = normalize_newlines(original)
            if normalized == original:
                continue
            with tempfile.NamedTemporaryFile(prefix="f4-msvc-response-", suffix=".rsp",
                                             delete=False) as stream:
                path = Path(stream.name)
                temporary.append(path)
                stream.write(normalized)
            command[index] = "@" + str(path)
        return subprocess.run(command, check=False).returncode
    finally:
        for path in temporary:
            path.unlink(missing_ok=True)


if __name__ == "__main__":
    sys.exit(run(sys.argv[1:]))
