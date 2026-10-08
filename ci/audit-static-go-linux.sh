#!/usr/bin/env bash
set -euo pipefail

binary="${1:?usage: audit-static-go-linux.sh <f4> [expected-arch]}"
expected_arch="${2:-}"
test -x "${binary}"

if [[ -n "${expected_arch}" ]]; then
    case "${expected_arch}" in
        amd64|x86_64) expected_machine="Advanced Micro Devices X86-64" ;;
        arm64|armv8) expected_machine="AArch64" ;;
        *)
            echo "error: unsupported expected Linux architecture: ${expected_arch}" >&2
            exit 2
            ;;
    esac
    actual_machine="$(readelf -W -h "${binary}" | sed -n 's/^[[:space:]]*Machine:[[:space:]]*//p')"
    if [[ "${actual_machine}" != "${expected_machine}" ]]; then
        echo "error: Go launcher machine is '${actual_machine}', expected '${expected_machine}'" >&2
        exit 1
    fi
    echo "Go launcher architecture: ${actual_machine}"
fi

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
