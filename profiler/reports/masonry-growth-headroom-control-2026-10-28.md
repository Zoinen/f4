# Bounded geometry growth headroom does not resolve cold-scroll spikes

A diagnostic adds 25% capacity, capped at 256 KiB, when an existing D3D11
geometry buffer must grow. It passes rendering/lifetime regressions but does
not consistently reduce allocation counts or display misses. The candidate is
not promoted. The active constant-160-Hz goal remains incomplete.

## Policy and correctness

The baseline is the previous initial-dynamic candidate, retaining guarded
geometry reuse, bounded staging uploads and uniform capacity retention. Both
arms use one executable; only F4_PROFILE_GROWTH_HEADROOM differs. The helper
affects growth at requested sizes of at least 8192 bytes. Initial allocation,
small geometry, upload length, draw length, buffer selection, retired-pool
budgets and other backends remain unchanged. Added capacity is min(size/4,
262144), rounded to four bytes, with an int-range overflow guard. This bounds
per-growth excess to 262147 bytes, not all active CPU/GPU memory.

A real native fixture starts with 80,000 vertex bytes, requests 88,000, then
96,000, 100,000 and 104,000. The control fails the new capacity assertion:
88,000 rather than 110,000. The candidate retains one native buffer after its
first growth and verifies later pixels, capacity, pointer, actual D3D11/DPR
1.75, physical scene origin (14,14) and unit transforms. Its rendered image is
saved and visually inspected. The complete candidate native suite passes
48 checks, zero failures/skips. Four import checks and the system-DLL audit
also pass. The exact source transformation reverses to the baseline.

## Capture and results

Downloads, 190 entries, Masonry density 152, actual 160 Hz / DPR 1.75.
Enabled/control/control/enabled, fresh processes and separate initially empty
application caches. Each capture has two ten-second rapid-wheel blocks;
standard media workers and hardware decoding remain enabled. The OS cache is
not flushed. Helpers retain output until the 45-second collection ends.
Focus, input count, target PID, capacity, feature-use and reuse/upload error
checks pass. No build, offline analysis or UI Automation observer runs during
the gestures. End process memory is sampled only after collection.

| Arm / Qt PID | Cold / next growths | Cold / next buffer creations | Cold / next missed refreshes | Cold / next max display gap |
| --- | --- | --- | --- | --- |
| Enabled / 110504 | 364 / 25 | 644 / 83 | 2 / 1 | 12.5117 / 12.4745 ms |
| Disabled / 93172 | 371 / 34 | 672 / 70 | 9 / 2 | 24.9774 / 12.5037 ms |
| Disabled repeat / 111632 | 270 / 33 | 552 / 88 | 8 / 5 | 18.7738 / 12.5156 ms |
| Enabled repeat / 101136 | 347 / 26 | 664 / 61 | 11 / 1 | 18.7503 / 12.4689 ms |

Counts cover render-thread native creation/producer events wholly inside
measurement windows, including fast calls. Misses mean display intervals above
9.375 ms. Both enabled cold growth counts exceed the second control's count;
the first enabled cold result is better but the reverse candidate is worse.
Different loading progress prevents causal comparison from counts alone.
There is no consistent evidence that this policy reduces real workload churn.

Coverage holds are zero, but every block fails the complete no-spike/motion
requirement. Maximum presentation-derived step errors range from 26.8794 to
231.2963 physical pixels. These observations are not physical scanout proof.
Last cache-entry counts are 190 / 207 / 204 / 209 respectively; these are
stored cache snapshots, not unique-file throughput or individual latencies.

| Arm | End private memory, MiB | Process peak pagefile, MiB |
| --- | ---: | ---: |
| Enabled | 1141.4 | 1746.4 |
| Disabled | 1199.3 | 1971.9 |
| Disabled repeat | 1210.2 | 1843.9 |
| Enabled repeat | 1244.4 | 2040.0 |

These whole-process samples do not isolate geometry memory or establish a
memory improvement. The added capacity increases allocation size by design;
pool budgets do not bound active resources or driver overhead.

## Remaining stalls and next action

The first enabled run's largest cold calls include staging-texture creation
6.2303 ms and a 6.2504 ms containing upload. The repeat records unchanged
constant-buffer host-write Map at 9.2199 ms, texture creation at 8.9866 and
8.7707 ms, and Present at 8.0104 ms. Its largest geometry-growth creation is
3.7200 ms for a 144,632-byte index buffer. The reverse control records a
17.4432 ms texture creation. Wall times alone do not establish lock ownership,
GPU execution or a driver defect.

Do not widen capacity further on this evidence. The hypothesis that nearby
size increases are the dominant source of growth is not supported by the live
counts. Next collect actual resource provenance/lifetimes and focus on texture
creation rather than repeating capacity heuristics. Current atlas-extraction
and standalone texture creation must be distinguished from descriptors alone;
previous atlas-aware shader regressions remain relevant and are not overridden
by this experiment.

The matching experiment snapshot retains sources, receipts, fixtures, rendered
output, native/import/DLL logs, compressed capture packets, target PresentMon
rows, memory sidecars and analyses with hashes. Diagnostic process trees were
stopped; the normal portable launcher and Qt host remain unchanged and alive.
No embedded-payload replacement, commit or push.

AI assistance has been used to create this output.
