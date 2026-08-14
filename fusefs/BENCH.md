# Mount performance: the baseline

Numbers taken with `fusefs/bench-all.sh` before the locking in `bridge.go` is
touched, because iteration 6 is gated on being able to show that a change
moved something. A number without its fixture and its link means nothing, so
both are recorded.

Fixture: one 8 MiB file, 500 small text files, one nested path. The same
content in all three sources, so the differences are the backend.

## 2026-08-13, f4 ab68a5f, Linux 6.8

| | local dir | archive (tar) | sftp, fast link | sftp, mobile link |
|---|---|---|---|---|
| sequential read (`dd`) | 0.01s | 0.02s | 0.60s\* | 9.45s |
| walk + read all (`tar -c`) | 0.01s | 0.01s | 0.01s\* | 2.37s |
| stat every entry (`ls -lR`) | 0.01s | 0.02s | 0.01s\* | 0.01s |
| search (`grep -rlI`) | 0.06s | 0.56s | 1.74s\* | 156.75s, failed |

\* the fast-link column was taken with the older 64 MiB fixture, so its
sequential read is not comparable with the rest of its column; the shape is.

## What the baseline already says

**Metadata is free, everywhere.** `ls -lR` over a remote host on a mobile
connection costs the same as over a local directory. The directory cache and
the kernel's attribute cache are doing exactly what they were put there for,
and there is nothing to win in that column.

**Bandwidth is bandwidth.** `dd` tracks the link and nothing else. No amount
of locking policy changes it.

**The search is the outlier, and it is not bandwidth.** On the mobile link,
`tar -c` reads every byte of the fixture in 2.37s while `grep` over the same
files takes 156s and then fails. Two orders of magnitude between two commands
reading the same data is not a slow link; it is something about how the mount
answers many small opens, and it is reproducible on that machine while being
absent on the fast one.

That is the thread worth pulling before touching any mutex: a lock that is
held per call cannot explain a 65x gap against a single-threaded reader, so
either the per-open cost is much higher than `tar` makes it look, or something
in the read path is doing far more work than the request needs.

## Counting what the mount actually asks for

`F4_FUSE_STATS=1` makes a mount tally every call it makes to its backend and
print the tally when it ends:

    F4_FUSE_STATS=1 ./f4 --mount sftp://user@host/path --foreground
    # in another shell: run the slow command, then ./f4 --umount <point>

    f4 fuse: VFS calls made by this mount
      ReadDir           2 calls          0s total       100µs each
      Open             20 calls          0s total         9µs each
      Stat             21 calls          0s total         7µs each
      ReadAt           20 calls          0s total         1µs each

A benchmark can only say that something is slow. This says which call is being
made and how many times, which is the part that says why — a backend answering
500 opens is a different problem from one answering 50 000 lookups, and from
the outside the two look identical.

For the search outlier the question it answers is precise: if `grep` produces
roughly the same call counts as `tar` and each call is slower, the problem is
in the calls; if it produces vastly more of them, the problem is that the read
path is doing work the request never asked for.

## Where this stopped, 2026-08-13

Confirmed on the mobile link with an 8 MiB fixture: `dd` 16.94s, `tar -c`
2.11s, `ls -lR` 0.01s, `grep` 241.70s. The search is not an artefact — it is
reproducible, it grows faster than the fixture, and `tar` reading the same
bytes over the same link is a hundred times quicker.

The counting run has not been done yet. That is the next action, and it is one
command twice:

    BENCH_KEEP=1 ./fusefs/bench-all.sh          # fixtures stay in place

    # run 1, in one shell:
    F4_FUSE_STATS=1 ./f4 --mount sftp://USER@HOST/PATH --foreground
    # in another: tar -cf /dev/null -C MOUNTPOINT . ; ./f4 --umount MOUNTPOINT
    # run 2: the same, with grep -rlI TODO MOUNTPOINT instead of tar

The tally prints in the first shell on unmount. Comparing the two answers the
question directly: similar call counts with slower calls means the cost is in
the calls; far more calls means the read path is doing work the request never
asked for. Nothing about the locking should be touched until that number
exists.

## Next

* Find out what the search actually does differently — per-file open cost, or
  a cache that expires under it, or an error it recovers from 500 times.
* Only then decide whether the single bridge lock is worth replacing, and
  re-run this script to say so with numbers.
* The benchmark has no concurrent case yet. The lock is about parallel load,
  so a fifth measurement — two readers at once — has to exist before the
  locking change can be judged at all.
