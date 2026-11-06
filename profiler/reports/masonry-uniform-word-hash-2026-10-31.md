# Masonry: bounded word-at-a-time uniform lookup

A word-at-a-time content hash makes the shared-uniform lookup primitive about
9.4 times faster in its synthetic benchmark and reduces median live material
preparation time. It does **not** solve the display spikes: one candidate run
misses two refreshes, while both baseline runs pass. The candidate remains
experimental, disabled by default, with no product rollout.

## Change and correctness

The [observer control](masonry-uniform-observer-control-2026-10-31.md) removed
uniform-snapshot bookkeeping from both arms of this comparison. The next
sampled hotspot was the serial FNV byte loop in shared-uniform acquisition.

The candidate changes only that acquisition's runtime bucket hash. Independent
64-bit lanes process payload words loaded with `memcpy`; a bounded copied tail
handles remaining bytes. It requires no additional CPU instruction-set feature,
alignment assumption, persistent key format or new dependency. The diagnostic
switch is `F4_PROFILE_UNIFORM_WORD_HASH`; its default remains the original hash.

Full size and byte comparisons remain mandatory after hashing. The 128-value
per-frame limit, 512-key table, eight-owner limit, frame epochs, immutable copied
keys, buffer ownership, uploads and collision fallback are unchanged. The
diagnostic content-observer table's own byte hash is also unchanged. All source
copies containing the modified shared-uniform inline definitions were rebuilt
consistently: renderer, main, native test and helper translation units.

New regressions cover selection of the requested mode, every bit in payloads
of length 1 through 256, unaligned addresses, exact end-of-readable-page buffers,
and equivalent table results across repeated queries and frame epochs. Existing
forced-collision, capacity, owner, buffer-copy and native lifecycle tests remain.

Verification:

- 21 helper tests passed in each mode, no failures/skips.
- 46 native tests passed in each mode on the actual 175% desktop.
- All 25 rendered PNGs are byte-identical between modes, including native text,
  thin strokes and vector fixtures.
- Embedded-only QML import test: 3 passed. Host DLL dependency audit passed.
- The portable host graph is retained, but this separately named diagnostic
  host is not a rebuilt/embedded Go payload. Normal F4 binaries are unchanged.

The first compile found duplicate definitions from mixing two copies of an
existing probe header. Consistent include resolution fixed that build failure;
the failed log is preserved. Installed Qt packages and SDK archives were not
modified.

## Hash benchmark

The corpus has 1,024 matrix-like payloads of lengths 64, 80, 112, 160, 192 and
256 bytes. Each trial performs 262,144 queries and consumes their checksums.
Eight trials per algorithm were collected in two helper processes, using
interleaved baseline/candidate ordering.

| Hash | Median ns/query | Range ns/query |
| --- | --- | --- |
| Original byte loop | 125.541 | 121.128–128.985 |
| Word lanes | 13.331 | 12.837–13.998 |

The ratio is 9.417. This measures only the hash primitive on synthetic inputs;
it is not a frame-time, full-cache-lookup or UI speedup claim.

## Live A/B/B/A comparison

The valid sequence was A1, B1r2, B2, A2. The original B1 ran out of space on D:
while serializing its final packet and produced a zero-byte JSON file. It is
invalid and excluded. Its remaining files, process identity and failure reason
were preserved. Only its exact owned diagnostic process tree was stopped.

Old diagnostic build folders were copied to C:, checked file-by-file by hash,
and exposed at their original paths through verified junctions. Only the checked
duplicate D: copies were retired. The capture free-space guard was increased
from 250 to 750 MiB, and B1r2 used a new case/cache identity. The valid sequence
therefore contains an interrupted attempt between A1 and B1r2, not four
uninterrupted captures.

All valid captures used the actual Downloads directory, all 192 entries,
Masonry density 152, 3840×2160, 160 Hz and DPR 1.75. Each included a verified
all-entry warm tour and two 30-second quarter-window-height middle-scroll
passes. Native foreground was stable, cache/renderer settings were fixed, and
the uniform-snapshot observer remained disabled in every arm. There was no
additional kernel/PMC logger or external-app affinity control.

| Run | Hash | Refresh misses by pass | Max display interval, ms | Strict middle result |
| --- | --- | --- | --- | --- |
| A1 | byte | 0 / 0 | 6.5351 | pass |
| B1r2 | word | 0 / 0 | 6.5344 | pass |
| B2 | word | 2 / 0 | 12.4674 | fail: displayed-motion continuity |
| A2 | byte | 0 / 0 | 6.4780 | pass |

The original gates are unchanged: display interval ≤9.375 ms, physical motion
error ≤2 px, native velocity error ≤.002 px, no coverage holds. Passing runs
satisfy all those checks. B2 fails at nonconsecutive displayed-motion frames;
no missing pair was discarded to force a pass. A sparse four-run series cannot
establish a reliable change in miss rate, but it clearly does not demonstrate
that the candidate eliminates misses.

## Live phase timings

These are elapsed phase durations over both measurement passes, not sampled
scheduled CPU time. A1 and B1r2 each have 9,601 complete measured frames; B2
and A2 have 9,600. Every frame has the expected 13 preparation-stage markers.
Hash acquisition occurs during material preparation, distinct from the earlier
geometry upload stage.

| Run | Overall preparation median / p99, ms | Alpha-material preparation median / p99, ms |
| --- | --- | --- |
| A1 | .3610 / 1.6401 | .2222 / .4795 |
| B1r2 | .3322 / 1.6656 | .1997 / .4534 |
| B2 | .3452 / 1.6071 | .2081 / .4527 |
| A2 | .3467 / 1.7279 | .2155 / .4664 |

Both word-hash runs have lower median alpha-material time than either baseline
in this series. Overall preparation p99 overlaps and does not improve uniformly.
The benchmark supports a faster primitive; the live measurements support a
modest material-stage benefit, not a general guarantee about frame tails.

B2's two missing-motion pairs are near presents 2422–2426. The nearby data
include a 12.1075 ms whole Qt frame with 1.3350 ms sync, 1.4745 ms preparation,
2.0435 ms recording and 1.3340 ms submission. Several phases inflate together;
the hash stage alone cannot account for that pattern. PresentMon also records
GPU completion and display delays in the preserved rows. No scheduler/graphics
event trace was captured here, so these observations do not prove whether
descheduling, GPU/driver work, compositor scheduling or another cause dominated
each missed refresh. A later CPU frame must not be assigned as the cause of an
earlier display gap merely because it is nearby in the table.

## Decision and remaining work

Retain the tested candidate for further CPU work, but leave it disabled by
default and do not roll it into the normal app on this evidence. Further tiny
hash changes are lower priority than a bounded, correlated capture of the
remaining multi-phase/delivery stall using the cleaner observer-disabled host.
The preceding scheduler trace showed that compositor misses can occur with F4
already GPU-ready; the current capture does not re-prove that cause for B2.

Cold wheel and verified-warm wheel were not revalidated here. The complete
normal-window workload, native SVG quality, alignment, motion and coverage
requirements remain outstanding; constant 160 Hz without spikes is not achieved.

## Preservation and final state

The [manifest](../experiments/masonry-uniform-word-hash-2026-10-31/manifest.json)
preserves sources, build/test logs, strict results, the invalid attempt and hash
receipts for raw captures. D: packet duplicates were retired only after checking
their preserved C: copies. No unrelated data was deleted.

All owned diagnostic apps/loggers are stopped. Normal Go PID 23488 and Qt PID
84780 remain alive. No commit or push was made. Future captures do not require
another readiness question.

> AI assistance has been used to create this output.
