#!/usr/bin/env bash
set -euo pipefail

host="${1:?usage: audit-portable-qt-linux.sh <f4-qt-host> [max-glibc] [expected-arch]}"
max_glibc="${2:-2.27}"
expected_arch="${3:-}"

test -x "${host}"

if [[ -n "${expected_arch}" ]]; then
    case "${expected_arch}" in
        amd64|x86_64) expected_machine="Advanced Micro Devices X86-64" ;;
        arm64|armv8) expected_machine="AArch64" ;;
        *)
            echo "error: unsupported expected Linux architecture: ${expected_arch}" >&2
            exit 2
            ;;
    esac
    actual_machine="$(readelf -W -h "${host}" | sed -n 's/^[[:space:]]*Machine:[[:space:]]*//p')"
    if [[ "${actual_machine}" != "${expected_machine}" ]]; then
        echo "error: portable Qt host machine is '${actual_machine}', expected '${expected_machine}'" >&2
        exit 1
    fi
    echo "Portable Qt host architecture: ${actual_machine}"
fi

needed="$(readelf -d "${host}" | sed -n 's/.*Shared library: \[\([^]]*\)\].*/\1/p')"
printf '%s\n' 'Qt host DT_NEEDED:' "${needed:-  (none)}"

forbidden='^(libQt[56]|libQWindowKit|libZoinGallery|lib(raw|tiff|png|jpeg|turbojpeg|heif|webp|de265|jbig|jasper|zstd|lzma)|lib(avcodec|avformat|avutil|avdevice|avfilter|swresample|swscale|postproc|pulse|pulse-simple|pulse-mainloop-glib|pulsecommon|asound|sndfile|openal|vpx|opus|vorbis|ogg|mp3lame|x264|x265|aom|dav1d|theora)|libstdc\+\+|libgcc_s)'
if printf '%s\n' "${needed}" | grep -Eiq "${forbidden}"; then
    echo "error: application-owned shared dependency remains in the portable Qt host" >&2
    printf '%s\n' "${needed}" | grep -Ei "${forbidden}" >&2
    exit 1
fi

if nm -D "${host}" 2>/dev/null | grep -Eq ' U (_Z|GLIBCXX|CXXABI)'; then
    echo "error: portable Qt host contains unresolved C++ runtime symbols" >&2
    nm -D "${host}" 2>/dev/null | grep -E ' U (_Z|GLIBCXX|CXXABI)' >&2
    exit 1
fi

highest_glibc="$(
    readelf --version-info --wide "${host}" 2>/dev/null |
        grep -o 'GLIBC_[0-9][0-9.]*' |
        sed 's/^GLIBC_//' |
        sort -Vu |
        tail -1
)"
if [[ -z "${highest_glibc}" ]]; then
    echo "error: portable Qt host has no auditable GLIBC symbol versions" >&2
    exit 1
fi
if [[ "$(printf '%s\n%s\n' "${highest_glibc}" "${max_glibc}" | sort -Vu | tail -1)" != "${max_glibc}" ]]; then
    echo "error: Qt host requires GLIBC_${highest_glibc}, baseline is GLIBC_${max_glibc}" >&2
    exit 1
fi
echo "Highest required glibc: ${highest_glibc} (allowed: ${max_glibc})"

if readelf -d "${host}" | grep -Eq '\((RPATH|RUNPATH)\)'; then
    echo "error: portable Qt host contains RPATH/RUNPATH" >&2
    readelf -d "${host}" | grep -E '\((RPATH|RUNPATH)\)' >&2
    exit 1
fi

if ! readelf -W -h "${host}" | grep -Eq '^[[:space:]]*Type:[[:space:]]+DYN([[:space:]]|$)'; then
    echo "error: portable Qt host is not a position-independent executable" >&2
    readelf -W -h "${host}" | grep -E '^[[:space:]]*Type:' >&2 || true
    exit 1
fi
if ! readelf -W -l "${host}" | grep -q 'GNU_RELRO'; then
    echo "error: portable Qt host is missing GNU_RELRO" >&2
    exit 1
fi
if ! readelf -W -d "${host}" | grep -Eq '\(FLAGS(_1)?\).*([[:space:]]BIND_NOW|[[:space:]]NOW([[:space:]]|$))'; then
    echo "error: portable Qt host is missing BIND_NOW" >&2
    exit 1
fi
stack_segment="$(readelf -W -l "${host}" | awk '$1 == "GNU_STACK" {print; exit}')"
if [[ -z "${stack_segment}" || "${stack_segment}" == *RWE* ]]; then
    echo "error: portable Qt host does not have a non-executable GNU_STACK" >&2
    printf '%s\n' "${stack_segment:-  (missing GNU_STACK)}" >&2
    exit 1
fi
echo "Portable Qt hardening: PIE, RELRO, BIND_NOW, and non-executable stack verified"

if nm -D "${host}" 2>/dev/null | grep -Eq ' (Qt|QWindowKit|ZoinGallery)[A-Za-z0-9_]*$'; then
    echo "error: portable Qt host exports application-owned dependency symbols" >&2
    exit 1
fi
