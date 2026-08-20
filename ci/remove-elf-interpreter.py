#!/usr/bin/env python3
"""Remove an inert ELF64 PT_INTERP program-header entry in place."""

from __future__ import annotations

import pathlib
import struct
import sys


PT_INTERP = 3
PT_NULL = 0


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: remove-elf-interpreter.py <elf>")
    path = pathlib.Path(sys.argv[1])
    data = bytearray(path.read_bytes())
    if data[:4] != b"\x7fELF" or data[4] != 2 or data[5] != 1:
        raise SystemExit(f"unsupported ELF format: {path}")

    phoff = struct.unpack_from("<Q", data, 32)[0]
    phentsize = struct.unpack_from("<H", data, 54)[0]
    phnum = struct.unpack_from("<H", data, 56)[0]
    removed = 0
    for index in range(phnum):
        entry = phoff + index * phentsize
        if entry + 4 > len(data):
            raise SystemExit(f"truncated ELF program header table: {path}")
        if struct.unpack_from("<I", data, entry)[0] == PT_INTERP:
            struct.pack_into("<I", data, entry, PT_NULL)
            removed += 1
    if removed != 1:
        raise SystemExit(f"expected one PT_INTERP entry, found {removed}: {path}")
    path.write_bytes(data)


if __name__ == "__main__":
    main()
