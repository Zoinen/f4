#!/bin/sh
# Measure a mount, so that "faster" is a claim somebody can check.
#
# The rest of iteration 6 replaces the single bridge lock with per-backend
# policy. That change is worth making only if it moves numbers, and the
# numbers have to exist before it, not after — otherwise the comparison is
# against a memory of how it felt.
#
# Usage:  fusefs/bench.sh SOURCE [f4-binary]
#
# SOURCE is anything f4 --mount accepts: a directory, an archive, a NetFox
# connection. The script mounts it read-only, times three ordinary programs
# against the mount, and unmounts. It changes nothing and writes nothing into
# the mount, so it is safe to point at a real host.

set -eu

SOURCE=${1:-}
F4=${2:-./f4}

if [ -z "$SOURCE" ]; then
	echo "usage: $0 SOURCE [f4-binary]" >&2
	exit 1
fi
if ! command -v "$F4" >/dev/null 2>&1 && [ ! -x "$F4" ]; then
	echo "$0: no f4 binary at $F4" >&2
	exit 1
fi

MNT=$("$F4" --mount "$SOURCE" --daemon)
if [ -z "$MNT" ]; then
	echo "$0: the mount did not come up" >&2
	exit 3
fi
echo "mounted $SOURCE at $MNT"
trap '"$F4" --umount "$MNT" >/dev/null 2>&1 || true' EXIT INT TERM

# The three shapes of load that matter, and what each one is sensitive to.
run() {
	label=$1
	shift
	printf '%s: ' "$label"
	start=$(date +%s.%N)
	"$@" >/dev/null 2>&1 || printf '(failed) '
	end=$(date +%s.%N)
	echo "$start $end" | awk '{printf "%.2fs\n", $2 - $1}'
}

# One big sequential read: bandwidth, and whether a transfer blocks everything.
BIG=$(find "$MNT" -type f -size +1M 2>/dev/null | head -n 1 || true)
# Everything below this line reads through the mount, so a slow backend is
# measured, not waited on: the search skips binaries rather than pulling the
# big file down a second time to decide it has no text in it.
if [ -n "$BIG" ]; then
	run "sequential read (dd)" dd "if=$BIG" of=/dev/null bs=1M
else
	echo "sequential read (dd): skipped, no file over 1 MiB in the mount"
fi

# Many small files: per-file overhead, and whether Lookup costs a round trip.
run "walk + read all (tar -c)" tar -cf /dev/null -C "$MNT" .

# Metadata storm: one Getattr per entry, plus whatever the kernel caches.
run "stat every entry (ls -lR)" ls -lR "$MNT"

# Reads with seeks in them, which is where spooling shows up.
if command -v rg >/dev/null 2>&1; then
	run "search (rg)" rg --files-with-matches --no-messages --binary TODO "$MNT"
else
	run "search (grep -r)" grep -rlI TODO "$MNT"
fi

echo "done; record these numbers before changing the locking, not after"
