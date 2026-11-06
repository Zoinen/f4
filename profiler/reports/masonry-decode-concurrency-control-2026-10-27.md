# Decode concurrency control: fewer cold misses, fewer prepared thumbnails

Serializing image read/decode jobs consistently reduces cold display misses
in two temporal comparisons, but also substantially reduces the thumbnail
frames stored during the captures. It does not meet the 160-Hz objective and
is not promoted. Preparation concurrency is a useful contributor to investigate,
not sufficient evidence that a particular decoder, driver lock or device is
the cause.

## Intervention and scope

GalleryRuntimeOptions.maxDecodeThreads defaults to four. DecodeManager retains
at least three specialized worker lanes (read, cache, probe), so setting that
option to one would not create a one-decoder control. A diagnostic-only copy of
DecodeManager instead limits active ImageRead/ImageDecode runners to one while
preserving four configured workers, metadata/cache/probe routing, source types,
task priorities and all supported media. No runner is discarded by the limit.
The patch reverses exactly to the current worktree source.

The same host runs with the new limit disabled and enabled. Fixed scalar
counters record the limit, observed concurrency, started jobs and blocked
admission decisions. The additional worker scan and per-blocked-candidate atomic
increments introduce diagnostic overhead; the limited arm records roughly
243,000-254,000 blocked admission decisions. This is not a zero-overhead
production scheduler design. The test does not isolate GPU-consuming work from
source I/O or CPU decode: it deliberately serializes both runner categories.

The host retains the preceding resource-identity probe, original atlas upload
path and production dependency archives. The rejected single-atlas-upload
object is absent. No production source, dependency archive or embedded payload
was changed.

## Checks before capture

A scheduler fixture queues six real ImageDecodeRunner subclasses with bounded
test work plus a cache runner. Default and limited invocations each pass three
cases including setup/cleanup. It verifies four retained workers, observed
concurrency of three versus one for these decode-only jobs, all six completions,
an empty queue and no active runners afterward. The limited case also verifies
cache progress before all decodes finish. Real captures can reach four heavy
jobs because the ImageRead category can occupy the dedicated read lane too.

The diagnostic host passes four import-isolation/compiled-host checks and the
system-DLL audit. No UI geometry is changed. This step does not claim a new
full native-rendering test run or a broad scheduler regression-suite pass.

## Matched cold captures

Order is default A, one-job B, one-job B2, default A2. All use the same host,
actual Downloads folder (190 entries), density 152, 2194 x 1186 logical window,
160-Hz screen and DPR 1.75, with default media behavior and separate empty
application cache roots. OS disk cache and Go metadata are not flushed. Each
has two ten-second rapid-wheel blocks; no builds/tests, offline analysis or
UI Automation observer run during input. Focus, input-count and recorder bounds
pass. The preparation observer's maximum callback stays below 58 microseconds.

| Arm / Qt PID | Block | Display gaps >9.375 ms | p99 interval | Maximum interval | Coverage holds |
| --- | ---: | ---: | ---: | ---: | ---: |
| A default / 77400 | 0 | 12 | 7.4869 ms | 18.9419 ms | 0 |
| A default / 77400 | 1 | 0 | 6.3108 ms | 6.4503 ms | 0 |
| B one job / 15756 | 0 | 3 | 6.4268 ms | 12.5230 ms | 0 |
| B one job / 15756 | 1 | 2 | 6.3735 ms | 12.5205 ms | 0 |
| B2 one job / 127400 | 0 | 4 | 6.3639 ms | 12.5906 ms | 1 |
| B2 one job / 127400 | 1 | 3 | 6.3425 ms | 18.7419 ms | 0 |
| A2 default / 63452 | 0 | 14 | 7.8454 ms | 18.8969 ms | 0 |
| A2 default / 63452 | 1 | 3 | 6.3443 ms | 18.7457 ms | 0 |

| Arm | Actual peak heavy jobs | Jobs started | Stored frames at finish | Pending at finish | Cache misses |
| --- | ---: | ---: | ---: | ---: | ---: |
| A | 4 | 523 | 195 | 0 | 813 |
| B | 1 | 345 | 128 | 0 | 1484 |
| B2 | 1 | 370 | 141 | 0 | 1343 |
| A2 | 4 | 545 | 215 | 0 | 772 |

These are prepared-frame/store counters, not counts of unique visible files or
proof of complete catalog coverage. Fewer stores and substantially more misses
are material signs of a preparation tradeoff. Pending zero does not imply every
catalog entry has a ready thumbnail; visibility-driven requests can be canceled
or replaced during rapid scrolling. Capture output alone does not quantify
per-visible-item latency. Do not claim acceptable thumbnail UX from these rows.

In B, a small standalone texture allocation still takes 7.8738 ms and atlas
updates take 5.2416/5.1612 ms. Thus serialization leaves graphics-call stalls,
and block 1 can still miss refreshes. This capture has no scheduler trace:
those are API wall times, not independently measured blocked durations.
The diagnostic motion analysis publishes all cases; it is not a strict pass.

## Decision and next evidence

Keep the limit experimental. The repeated cold improvement is consistent with
concurrent preparation contributing to stalls, but source/decode/output workload
and diagnostic scheduling cost also change. Do not infer a specific GPU lock
owner, or disable video/hardware decoding. The existing four-worker production
policy remains in force.

Next capture call stacks for the known blocked D3D11 intervals and identify
device/context ownership. That can distinguish waits inside driver/resource
allocation from application synchronization and work on other contexts. Use an
owned bounded scheduler trace and preserve the actual cold workload; do not
substitute a warm rendering-only benchmark or relax presentation gates.

Only the exact owned decode diagnostic launcher tree was closed afterward.
Normal worktree launcher 43936 / Qt 119460 remain running. No commit/push.

Evidence: `profiler/experiments/masonry-decode-concurrency-control-2026-10-27/`.

> AI assistance has been used to create this output.
