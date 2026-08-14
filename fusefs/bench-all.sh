#!/bin/sh
# Collect the iteration 6 baseline in one go: build f4, make identical test
# data in three places, mount each of them, and write one report.
#
# The three sources deliberately hold the same files, so the numbers differ
# because of the backend and not because of what is in it.
#
# Usage:  fusefs/bench-all.sh [ssh-host] [remote-dir]
#
# With no ssh host the remote part is skipped and the local ones still run.

set -eu

HOST=${1:-www-test.runcity.org}
RDIR=${2:-claude/f4bench}
# How big the one large file is. The default is small on purpose: the point of
# that file is to measure bandwidth, and on a slow link a 64 MiB file measures
# the user's patience instead. Raise it on a fast connection.
MB=${BENCH_MB:-8}
HERE=$(cd "$(dirname "$0")/.." && pwd)
WORK=${TMPDIR:-/tmp}/f4bench.$$
REPORT=$HERE/bench-report.txt
F4=$HERE/f4

log() { echo "$@" | tee -a "$REPORT"; }

mkdir -p "$WORK"
: > "$REPORT"
log "f4 fusefs benchmark baseline"
log "date:   $(date -u '+%Y-%m-%d %H:%M:%SZ')"
log "system: $(uname -srm)"
log "fixture: ${MB} MiB big file, 500 small files"
log ""

# --- 1. the binary ------------------------------------------------------
echo "building f4..."
(cd "$HERE" && go build -o f4 .)
log "f4:     $("$F4" --version 2>/dev/null | head -n 1 || echo "$(cd "$HERE" && git rev-parse --short HEAD)")"
log ""

# --- 2. the fixture -----------------------------------------------------
# One big file for bandwidth, many small ones for per-file overhead. Small
# enough to make over an ssh link in a few seconds.
make_fixture() {
	dir=$1
	mkdir -p "$dir/many"
	dd if=/dev/urandom of="$dir/big.bin" bs=1M count="$MB" 2>/dev/null
	i=0
	while [ $i -lt 500 ]; do
		printf 'file %d\nTODO marker\n' "$i" > "$dir/many/f$i.txt"
		i=$((i + 1))
	done
	mkdir -p "$dir/nested/a/b/c"
	printf 'TODO deep\n' > "$dir/nested/a/b/c/deep.txt"
}

echo "making local fixture..."
LOCAL=$WORK/fixture
make_fixture "$LOCAL"

echo "making archive..."
ARCHIVE=$WORK/fixture.tar
(cd "$WORK" && tar -cf fixture.tar fixture)

# --- 3. the same thing on the far end -----------------------------------
REMOTE=""
if [ -n "$HOST" ] && ssh -o BatchMode=yes -o ConnectTimeout=10 "$HOST" true 2>/dev/null; then
	echo "making remote fixture on $HOST..."
	# Generated on the host rather than uploaded: 64 MiB over the link
	# would measure the upload, which is not what we are asking about.
	ssh -o BatchMode=yes "$HOST" "
		set -e
		rm -rf '$RDIR'
		mkdir -p '$RDIR/many' '$RDIR/nested/a/b/c'
		dd if=/dev/urandom of='$RDIR/big.bin' bs=1M count=$MB 2>/dev/null
		i=0
		while [ \$i -lt 500 ]; do
			printf 'file %d\nTODO marker\n' \$i > '$RDIR'/many/f\$i.txt
			i=\$((i + 1))
		done
		printf 'TODO deep\n' > '$RDIR/nested/a/b/c/deep.txt'
		pwd
		id -un
	" > "$WORK/remote-info" 2>/dev/null || true
	RHOME=$(sed -n 1p "$WORK/remote-info" 2>/dev/null || echo "")
	RUSER=$(sed -n 2p "$WORK/remote-info" 2>/dev/null || echo "")
	if [ -n "$RHOME" ]; then
		# The URL is assembled rather than pasted together: $RHOME is
		# already absolute, so a slash in front of it makes the double
		# slash that f4 then reads as an empty first path element. The
		# user has to be in there too — ssh takes it from its own config,
		# and f4 has no way to see that.
		case "$HOST" in
		*@*) REMOTE="sftp://$HOST$RHOME/$RDIR" ;;
		*)   REMOTE="sftp://${RUSER:+$RUSER@}$HOST$RHOME/$RDIR" ;;
		esac
	fi
else
	log "remote: skipped (no passwordless ssh to ${HOST:-<none>})"
fi

# --- 4. measure ---------------------------------------------------------
bench_one() {
	name=$1
	source=$2
	log ""
	log "=== $name"
	log "    source: $source"
	if sh "$HERE/fusefs/bench.sh" "$source" "$F4" >> "$REPORT" 2>&1; then
		:
	else
		log "    (this source did not finish; the lines above say how far it got)"
	fi
}

bench_one "local directory" "$LOCAL"
bench_one "archive (tar)" "$ARCHIVE"
if [ -n "$REMOTE" ]; then
	bench_one "sftp host" "$REMOTE"
fi

# --- 5. tidy up ---------------------------------------------------------
rm -rf "$WORK"
if [ -n "$REMOTE" ]; then
	ssh -o BatchMode=yes "$HOST" "rm -rf '$RDIR'" 2>/dev/null || true
fi

echo ""
echo "report written to $REPORT"
echo "send that file back; it is the baseline the locking change is measured against"
