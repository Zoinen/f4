#!/usr/bin/env bash
set -euo pipefail

host="${1:?usage: audit-portable-qt-linux.sh <f4-qt-host> [max-glibc]}"
max_glibc="${2:-2.27}"

test -x "${host}"

needed="$(readelf -d "${host}" | sed -n 's/.*Shared library: \[\([^]]*\)\].*/\1/p')"
printf '%s\n' 'Qt host DT_NEEDED:' "${needed:-  (none)}"

forbidden='^(libQt[56]|libQWindowKit|libZoinGallery|lib(raw|tiff|png|jpeg|turbojpeg|heif|webp|de265|jbig|jasper|zstd|lzma)|libstdc\+\+|libgcc_s)'
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

if nm -D "${host}" 2>/dev/null | grep -Eq ' (Qt|QWindowKit|ZoinGallery)[A-Za-z0-9_]*$'; then
    echo "error: portable Qt host exports application-owned dependency symbols" >&2
    exit 1
fi
