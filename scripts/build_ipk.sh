#!/bin/sh
# Builds an OpenWrt .ipk (opkg package) around one f4 binary, without the
# OpenWrt SDK: an ipk is a gzip'd tar of debian-binary, control.tar.gz and
# data.tar.gz. The binary is static, so the package depends on nothing.
#
# Usage: build_ipk.sh BINARY VERSION OPKG_ARCH OUTPUT.ipk
# OPKG_ARCH is the architecture name the device lists in /etc/opkg.conf or
# `opkg print-architecture` (mipsel_24kc, aarch64_generic, x86_64, ...); a
# package installs only where it matches.
set -eu
[ $# -eq 4 ] || { echo "usage: $0 BINARY VERSION OPKG_ARCH OUTPUT.ipk" >&2; exit 2; }
bin=$1 version=$2 arch=$3 out=$(realpath -m "$4")
mkdir -p "$(dirname "$out")"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

mkdir -p "$work/data/usr/bin" "$work/control"
install -m 0755 "$bin" "$work/data/usr/bin/f4"
size=$(stat -c %s "$bin")
cat > "$work/control/control" <<CTL
Package: f4
Version: $version
Depends:
Section: utils
Architecture: $arch
Installed-Size: $size
Maintainer: unxed <https://github.com/unxed/f4>
Homepage: https://github.com/unxed/f4
Description: efficient and cozy two-panel file manager (extra-lite build)
CTL

printf '2.0\n' > "$work/debian-binary"
tar -C "$work/control" --owner=0 --group=0 -czf "$work/control.tar.gz" .
tar -C "$work/data" --owner=0 --group=0 -czf "$work/data.tar.gz" .
tar -C "$work" -czf "$out" ./debian-binary ./control.tar.gz ./data.tar.gz
echo "$out: $(stat -c %s "$out") bytes"
