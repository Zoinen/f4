# Masonry constant-buffer burst reserve — 2026-10-29

## Result

A diagnostic candidate removes almost all measured constant-buffer pool misses,
but does not establish smoother overall presentation. It remains disabled by
default and is not promoted into the portable application. Constant 160 Hz,
zero display misses and maximum motion error of two physical pixels remain
unachieved. No content, source-loading priority, vector readiness, viewport,
animation or rendering fidelity was reduced to improve these numbers.

## Implementation and regression

The preceding scheduler trace showed a 5.1945-ms synchronous allocation of a
1,024-byte constant buffer. The pool contained two ready buffers per class,
while observed frames requested thirteen 256-byte and three 1,024-byte buffers.

The candidate adds fourteen 256-byte entries and two 1,024-byte entries: sixteen
and four ready buffers respectively. Reserved backing grows by 5,632 bytes,
from 25,427,440 to 25,433,072 bytes, below the unchanged 32-MiB limit. Device and
resource descriptor validation, exclusive fresh backing, round-robin worker
creation, concurrent-creation support checks and owner-thread adoption remain.
The worker never calls the immediate context.

Both arms use the same executable. The environment flag
`F4_PROFILE_UNIFORM_BURST_RESERVE` selects 116 active entries versus the original
100. Worker and acquisition loops use the active count, so the control does not
scan sixteen inactive entries each time. Native/backend and main translation
units use the same variant header and compatible structure layout. Existing
video-producer probe layouts are unchanged.

The new regression waits for all configured backing to exist, verifies actual
native descriptors and total storage, and checks capacity for the observed
simultaneous bursts. It fails in the control at thirteen ready 256-byte buffers
and passes with the candidate. Candidate pool tests: 10 passed; existing control
pool tests: 9 passed. They include GPU readback, ranged constant-buffer binding,
descriptor rejection, ownership and shutdown checks.

Broader verification: 48 native rendering tests and 27 shader tests pass at
actual DPR 1.75 without skips; four embedded import checks pass; the Windows
system-DLL-only audit passes. One initial broader run omitted the native-renderer
test flag and uniform-capability experiment flag, producing a failed expectation
and skipped coverage. That log is retained as
`native-tests-incomplete-environment.txt`; it is not counted as passing coverage.
The corrected run explicitly enables native rendering and asserts DPR 1.75.

## Four accepted live captures

Actual display: 3840×2160, 160 Hz, DPR 1.75. Downloads has 191 entries; Masonry
density is 152. Each process gets a fresh application gallery cache and performs
two ten-second 100-Hz boundary-bounce wheel blocks. Order is on, off, off, on.
All display, focus, input-count, probe-capacity and feature guards pass. No builds,
offline analysis or UI Automation observer run during gestures.

| Arm | Inputs | Uniform pool hits / misses | Preparation p99, blocks 0 / 1 | Preparation max, blocks 0 / 1 | Display misses, blocks 0 / 1 |
| --- | ---: | ---: | ---: | ---: | ---: |
| On | 1670 | 352 / 0 | 3.6259 / 3.6916 ms | 6.8746 / 9.8992 ms | 10 / 5 |
| Off | 1704 | 291 / 63 | 3.1654 / 3.9541 ms | 7.8733 / 11.6348 ms | 7 / 4 |
| Off repeat | 1670 | 314 / 54 | 3.5818 / 4.0529 ms | 8.0484 / 8.8717 ms | 8 / 9 |
| On repeat | 1674 | 356 / 1 | 3.1498 / 4.4244 ms | 6.6822 / 8.2052 ms | 7 / 6 |

Pool counters cover each process, not only the measurement windows. All devices
report zero creation errors and unsupported acquisitions. Peak reserved backing
matches each arm's declared bound.

Display misses use the unchanged interval threshold of 9.375 ms. Display maxima
are 18.7483 / 18.7005 ms (on), 14.0322 / 18.5916 ms (off),
12.5196 / 18.6094 ms (off repeat), and 18.7757 / 12.4964 ms (on repeat).
There are no coverage holds in these eight blocks, but maximum motion error is
107.6730 / 193.4802, 117.3132 / 109.2016, 94.7807 / 78.6711, and
95.6546 / 96.6909 physical pixels respectively. Every block fails the existing
two-pixel maximum. Median display intervals near 6.25 ms do not prove success.

Final pending source requests are 1, 12, 5 and 11. These are cold/loading runs,
not verified warm comparisons; source residency differs. The original 5-ms
constant-buffer allocation does not recur in either control, so its elimination
cannot be claimed from this experiment. Miss reduction is demonstrated; a
causal overall frame-time improvement is not.

## Remaining costs and next evidence

In the first candidate, an 8.7558-ms DEFAULT texture creation has actual size
153×273, BGRA8, one mip and shader-resource binding. Another staging texture
creation takes 4.7700 ms. A uniform DISCARD map reaches 6.1642 ms.

The candidate repeat records a 9.0096-ms staging texture creation for a
1,024×512 BGRA8 surface, nested within a 9.0875-ms update wrapper. Its uniform
DISCARD map reaches 4.6904 ms. The repeated control also records a 5.2284-ms
geometry NO_OVERWRITE map. These are measured API wall times; these captures
have no scheduler trace and do not by themselves prove CPU execution versus
blocking inside each call. Nested wrapper durations must not be added together.

Largest preparation frames continue to distribute work across node traversal,
render-list updates, material preparation and uploads. The controls retain their
buffer-allocation fallback without reproducing the prior 5-ms buffer tail;
unrelated driver stalls and loading vary across processes.

The next comparison must establish a fully loaded, repeatable route while
retaining all real content, then distinguish steady-state CPU preparation from
allocation/upload stalls during cold loading. The existing diagnostic
`warmScene` mode changes item visibility and is unsuitable for that acceptance
claim. Verified warm wheel and quarter-height middle scrolling remain required,
along with cold/loading acceptance. No staging-texture pooling change, driver
setting, process affinity, QoS or system power-plan change is implemented here.

## Reproducibility and rollout

Sources, build receipts, tests, analysis, capture methods, actual-display records
and screenshots are preserved in
`profiler/experiments/masonry-uniform-burst-reserve-2026-10-29`.
Large raw traces and compiled outputs are external hash-addressed evidence.
This is a worktree-specific diagnostic host override; the normal application
remains running. No production payload was replaced, no commit or push made.
The goal remains active.
