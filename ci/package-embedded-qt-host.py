#!/usr/bin/env python3
"""Create the deterministic payload consumed by embedded_qt_host_payload.go."""

from __future__ import annotations

import argparse
import gzip
import pathlib
import shutil


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("host", type=pathlib.Path)
    parser.add_argument(
        "output",
        nargs="?",
        type=pathlib.Path,
        default=pathlib.Path("embedded/f4-qt-host.gz"),
    )
    args = parser.parse_args()

    if not args.host.is_file():
        raise SystemExit(f"Qt host executable is missing: {args.host}")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temporary = args.output.with_suffix(args.output.suffix + ".tmp")
    with args.host.open("rb") as source, temporary.open("wb") as raw_output:
        with gzip.GzipFile(
            filename="f4-qt-host",
            mode="wb",
            compresslevel=9,
            fileobj=raw_output,
            mtime=0,
        ) as compressed:
            shutil.copyfileobj(source, compressed, length=1024 * 1024)
    temporary.replace(args.output)


if __name__ == "__main__":
    main()
