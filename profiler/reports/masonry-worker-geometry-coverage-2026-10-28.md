# Full geometry preparation reduces creation tails; constant-buffer creation remains

Extending worker preparation across the geometry-buffer size range removes
the arbitrary size cutoff responsible for the earlier 50,952-byte stall.
Both full-range enabled captures have render-thread eligible geometry allocation
wrappers below 0.37 ms, compared with control calls up to 7.19 ms. Overall
display results remain mixed, so neither variant is promoted. A remaining
12.2859 ms creation is a 1024-byte constant buffer, outside the geometry pool.
Constant 160 Hz and the strict motion gate remain incomplete.

## Two measured variants

The prior 64 KiB–4 MiB worker prototype is preserved. A separate variant lowers
the cutoff to 8 KiB, adding 8/16/32 KiB classes. This has 40 reserve slots and
33,521,664 bytes of summed native ByteWidth capacity. The enabled captures
adopt all eligible requests, but still record a 7.0346 ms creation of a
1092-byte index buffer and a 3.2503 ms creation of a 6624-byte index buffer.
These descriptors motivate removing the geometry cutoff rather than repeatedly
choosing a new threshold.

The full variant prepares powers of two from 4 bytes through 4 MiB, two slots
per class for exclusive vertex and exclusive index bindings. Its 84 slots sum
to 33,554,416 bytes, sixteen bytes below the unchanged 32 MiB bound. This bounds
reserve buffer capacity, not driver overhead/residency, adopted active buffers,
or all process memory. The bound remains per eligible device (at most 32 device
records); each live capture has one eligible device. Adopted capacity stays
at least the requested size and strictly below twice it. Larger-than-4-MiB and
non-geometry requests keep the original path.

Both variants preserve the fresh-buffer ownership model: no used GPU buffer
returns to the reserve. A worker only creates resources; the owner thread
retains context operations and the existing DISCARD/NO_OVERWRITE guards. A miss
does not wait for the worker. The reserve refills after adoption and joins on
device destruction. Driver concurrent-creation support is required. No UI
layout, QML, draw counts, logical/shadow buffer sizes, or media worker count
changes. Native descriptors reflect adopted physical capacity.

## Verification

Each variant passes five pool tests, including setup and cleanup. Real D3D11
readbacks cover both buffer bindings and all tested sizes, including 50,952
bytes; the full variant adds 4, 6, 1092 and 6624 bytes. Tests verify descriptor
eligibility, data, capacity bound, invalid sizes, wrong-thread rejection and
resource/worker shutdown. Each previous cutoff fails the new lower-bound
eligibility regression with its own ABI-compatible old header and calls object.
No old/new Device struct layouts are mixed in a test executable.

Each candidate passes 47 native scene checks and 27 rendered shader checks
at actual DPR 1.75, plus four isolated embedded-import checks and the Windows
dependency audit. There are zero skipped native checks or failed reuse maps.
There is no claim of representative scrolling from these tests alone.

## Live comparison

Real Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75.
Each variant runs enabled/control/control/enabled, with separate initially
empty application caches and two ten-second rapid-wheel blocks per process.
Same executable per comparison; only worker preparation differs. Earlier
unpromoted rendering/backend experiments remain identical in all arms. These
are not direct comparisons with the unchanged normal portable application.
No tests, offline analysis or UI Automation observer runs during gestures.

| Variant / arm / Qt PID | Hits / eligible misses | Cold / next missed refreshes | Cold / next maximum display interval |
| --- | ---: | ---: | ---: |
| 8 KiB enabled / 100984 | 101 / 0 | 10 / 0 | 12.5669 / 7.9600 ms |
| 8 KiB control / 90756 | Disabled | 7 / 1 | 14.3286 / 12.4609 ms |
| 8 KiB control repeat / 11672 | Disabled | 6 / 1 | 18.7427 / 12.5030 ms |
| 8 KiB enabled repeat / 75884 | 88 / 0 | 4 / 6 | 12.6298 / 12.6656 ms |
| Full enabled / 116192 | 898 / 126 | 8 / 1 | 12.5940 / 12.5068 ms |
| Full control / 109768 | Disabled | 7 / 6 | 12.6353 / 12.5872 ms |
| Full control repeat / 26692 | Disabled | 5 / 1 | 13.1905 / 12.5090 ms |
| Full enabled repeat / 99344 | 940 / 127 | 1 / 2 | 12.5718 / 12.5141 ms |

Misses mean display intervals above 9.375 ms wholly within gesture windows.
Reserve counters cover the whole recorded process. Full-range supply misses
fall back synchronously; they are not hidden waits or dropped content. Both
variants remain within capacity budgets and have zero worker creation errors.

| Full-range arm | Cold / next CPU render p99 | Cold / next CPU render max | Cold / next max eligible geometry allocation wrapper |
| --- | ---: | ---: | ---: |
| Enabled | 6.0285 / 3.3231 ms | 14.4101 / 6.2017 ms | 0.0775 / below 0.05 ms |
| Control | 6.0474 / 5.7640 ms | 9.6076 / 11.2086 ms | 6.2209 / 7.1878 ms |
| Control repeat | 5.1194 / 3.7808 ms | 9.7177 / 7.5970 ms | 0.9408 / below 0.05 ms |
| Enabled repeat | 5.0473 / 3.6147 ms | 8.5572 / 9.6557 ms | 0.3614 / below 0.05 ms |

The recording threshold is 50 microseconds: absent wrapper records are below
that threshold, not zero-time operations. The wrapper includes reserve adoption;
its resource record does not by itself prove a new native creation. Eligible
wrapper filtering joins owner-thread records, native resource descriptors and
matching times, and requires exclusive vertex/index bindings. Worker creations
have a separate API/producer label.

Full-range workers create 982/1024 buffers over their lifetimes, including the
84-slot initial reserve, with total native creation wall time 36.5196/23.7100 ms.
This increases object-creation concurrency; it does not prove absence of driver
contention. Loading snapshots store 196/205/187/203 cache entries respectively,
not a unique-file throughput measure. End private memory is
1119.82/1160.11/1119.19/1212.27 MiB; process peak pagefile is
1749.09/1694.13/2085.66/1892.28 MiB. These variable whole-process measurements
do not establish loading or memory neutrality.

The earlier 8 KiB captures store 191/206/198/185 entries, with end private
memory 1192.92/1148.13/1185.01/1184.93 MiB and peak pagefile
1772.88/1964.11/2043.78/1897.15 MiB. Neither this comparison nor the full-range
comparison consistently improves all cold/warm frame metrics. Motion analyses
have zero coverage holds, but no complete run meets the no-miss/motion goal.

## Remaining tails and next action

The full enabled cold run records a 12.2859 ms creation with bind flag 4,
ByteWidth 1024: a constant buffer, not an uncovered geometry buffer. Its
repeat's largest creation is also a constant buffer, 1536 bytes at 2.4896 ms.
Other enabled tails include geometry DISCARD at 6.0566 ms, Present at 2.6964 ms
and IASetVertexBuffers at 2.5238 ms. Removing geometry creation tails is
therefore insufficient for smooth whole frames.

Next investigate preparing fresh constant buffers without enlarging the
reserve bound, and separately improve evidence for safe reuse when the latest
frame marker is pending. Any extension must preserve binding limits, complete
CPU/GPU data tests and old queued reads. No such extension or completion-guard
change has been implemented in these variants.

Only exact worktree diagnostic launcher trees were stopped. The normal app
remains unchanged (Go PID 43936, extracted host PID 119460). No payload rollout,
commit or push occurred. The matching snapshot retains both variants, failing
baseline regressions, accepted live packets, per-variant motion outputs,
memory/PresentMon data, and external object/executable hashes.
