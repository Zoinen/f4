#!/usr/bin/env bash
set -euo pipefail

binary="${1:?usage: audit-static-go-linux.sh <f4>}"
test -x "${binary}"

if readelf -l "${binary}" | grep -q 'INTERP'; then
    echo "error: Go launcher has an ELF interpreter" >&2
    readelf -l "${binary}" | grep 'INTERP' >&2
    exit 1
fi
if readelf -d "${binary}" 2>/dev/null | grep -q 'NEEDED'; then
    echo "error: Go launcher has dynamic dependencies" >&2
    readelf -d "${binary}" | grep 'NEEDED' >&2
    exit 1
fi
echo "Go launcher is a static ELF with no interpreter or DT_NEEDED entries"
