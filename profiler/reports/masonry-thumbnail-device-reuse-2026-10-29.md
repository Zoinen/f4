# Masonry: thumbnail D3D11 device reuse, alone and with heap isolation

## Outcome

The goal of constant 160 Hz without spikes remains **unachieved**. Two new
matched four-capture experiments are complete:

- Device reuse alone: 20 control versus 15 candidate display intervals longer
  than 9.375 ms. A candidate block has no such intervals, but another reaches
  18.4324 ms. All blocks fail the 2-physical-pixel motion-error limit.
- Device reuse plus the earlier private heap: 23 control versus 17 candidate
  intervals longer than 9.375 ms. Candidate preparation reaches 13.7892 ms and
  display intervals reach 18.8095 ms. All blocks fail the motion-error limit.

The experiments shorten the sampled geometry-update tails, but neither proves
the requested end state or a reliable improvement in worst-case whole-frame
timing. Neither is promoted. No production binary, embedded payload, dependency
package or application source was replaced. No commit/push was made.

The preceding evidence is documented in
`masonry-av-private-heap-and-device-creation-2026-10-29.md`. That trace associates
the render thread's graphics-allocation wait with a thumbnail worker creating
a new D3D11 device. The current work tests avoiding those repeated creations
while retaining hardware video decoding.

## Implementation

Diagnostic host: `artifacts/f4-qt-host-av-thumbnail-device-reuse-20261029.exe`.
Device-only evidence: `artifacts/av-thumbnail-device-reuse-20261029/`.
Combined evidence: `artifacts/combined-av-thumbnail-20261029/`.
Frozen sources/results and artifact hashes:
`profiler/experiments/masonry-thumbnail-device-reuse-2026-10-29/`.

The exact locally installed FFmpeg `hwcontext.c` is compiled with only the
definition of `av_hwdevice_ctx_create` renamed. A strong-linked wrapper owns
the public symbol and delegates to that original implementation for fresh
devices and every ineligible request. Both implementations are present in the
host map; the archive's unmodified `hwcontext.o` is not linked a second time.
The original source and configuration hashes are retained in
`source-contract.json`.

An RAII scope in the diagnostic `VideoThumbnailRunner::run()` marks its thread
eligible until the runner returns, restoring the previous thread-local value
on every return. Nested scope restoration is tested. Viewer and other
non-thumbnail threads remain unmarked.

Reuse requires **all** of: the diagnostic flag, a marked thread, D3D11VA type,
null device name, null options and zero flags. The cache holds at most one
reference-counted default device. Every successful request receives its own
`AVBufferRef`; codec and frame contexts remain independent. Explicit adapter,
options, nonzero flags and other device types bypass the cache.

A mutex serializes cache creation and replacement. Device health is queried
through `GetDeviceRemovedReason`; an invalid cached reference is released
before a fresh creation, while existing clients retain their own references.
Creation failures are not cached. Failure to pin a fresh successful device
does not discard the caller's result; failure to create a reference on a cache
hit returns ENOMEM without corrupting the cached reference.

The cache retains its one device for process lifetime. It does not introduce
an eviction timer or a shutdown-order dependency on Qt static destructors.
The regression helper can clear it explicitly after clients finish. Its 184
tracked FFmpeg bytes in combined live captures do **not** represent all native
GPU/driver resources retained by the device.

The locally inspected FFmpeg D3D11 implementation already provides a device
mutex and enables `ID3D10Multithread` protection. Those mechanisms remain
unchanged. Sharing this device does not share decoder or frame-pool state.

## Correctness verification

- Device contract/lifetime tests: 4 control and 9 candidate groups pass.
  These cover non-thumbnail bypass; distinct result references; shared device
  identity; nested scope restoration; adapter/options/flags/type bypass;
  invalidation with older clients alive; allocation/reference failure;
  retry after creation failure; failure to pin a freshly created device;
  and eight simultaneous requests producing one cached device.
- Invalidation is **simulated**, exercising replacement and retained-client
  ownership. No physical GPU reset or hot removal was induced. Real device
  removal, adapter changes and prolonged playback still need further testing
  before any production adoption.
- Hardware decoding: all six real Downloads videos, five frames per video,
  three sequential passes, followed by eight simultaneous decoder contexts.
  Every received frame is asserted to be D3D11-backed; there is no software
  fallback in the test. Visible pixels are transferred for comparison and
  hashes match the control for all 26 results. H.264 and HEVC are exercised.
  The candidate uses one device creation and 25 cache hits; the control makes
  26 fresh device requests.
- The same hardware test is separately linked with private-heap allocation
  primitives and run with both features enabled. All 26 pixel comparisons
  match, and all tracked allocations are freed after clearing the cache.
- Actual DPR 1.75 renderer tests: 48 passed, zero failed/skipped.
- Shader rendered-pixel and scene-transform tests: 27 passed, zero
  failed/skipped.
- Installed-Qt-independent QML imports: 3 passed, zero failed/skipped.
- The host passes the Windows portable-dependency audit. No portable payload
  rollout is claimed.

No static UI layout is modified. Source transformations reverse exactly apart
from the explicit wrapper, thread scope and diagnostic snapshots. The normal
worktree application remains alive, unchanged.

## Capture method

All eight live captures use the same native host and real Downloads folder
with 191 entries, Masonry density 152, native text, actual 3840×2160 / 160 Hz /
DPR 1.75, and two ten-second rapid-wheel blocks. Existing viewport, content,
resource bounds, input-focus, display and error guards are retained.
Fresh isolated gallery caches are used in every case; the OS file cache is
not flushed. The normal cold-start scrollbar method is used without a prior
warm tour. These are **cold-loading comparisons**, not verified-warm results.

Device-only cases are `real-scroll-av-device-{a1,b1,b2,a2}-20261029-wheel-masonry`,
with cache suffixes `zza` through `zzd`. A disables both features; B enables
device reuse only. Combined cases are
`real-scroll-combined-av-{a1,b1,b2,a2}-20261029-wheel-masonry`, with cache suffixes
`zze` through `zzh`. A disables both; B enables both. Each comparison uses ABBA
order. The combined comparison requires no new host build.

No tests, builds, ETW stacks, UI Automation observers or offline analyses run
during the gestures. PresentMon is case-owned and stopped after each capture.
The fine unmap probe and all other diagnostic features are matched in both
arms. Preparation durations are elapsed wall time, not pure scheduled CPU.

## Device reuse alone

All timings below are milliseconds. Misses count display intervals >9.375 ms.

| Arm/block | Preparation median | Preparation p99 | Preparation max | Display max | Misses | Max motion error, physical px |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1/0 | 0.82495 | 3.3251 | 12.1881 | 18.7642 | 11 | 210.695 |
| A1/1 | 0.61515 | 2.6722 | 6.8598 | 12.4722 | 1 | 7.881 |
| B1/0 | 0.77255 | 3.2382 | 5.7720 | 6.6300 | 0 | 6.788 |
| B1/1 | 0.68885 | 4.2744 | 8.4484 | 18.4324 | 8 | 122.026 |
| B2/0 | 0.72630 | 3.6569 | 5.3292 | 12.4802 | 3 | 60.822 |
| B2/1 | 0.69670 | 3.7471 | 7.4495 | 12.6040 | 4 | 91.906 |
| A2/0 | 0.78115 | 3.4411 | 5.0618 | 12.5382 | 5 | 56.615 |
| A2/1 | 0.65070 | 3.3161 | 7.8333 | 12.5694 | 3 | 89.920 |

B1/B2 record one device creation with 19/20 hits, no bypasses, invalidations,
creation failures or pin failures, and a cached device at snapshot time.
Control runs record 23/18 bypasses. These are cumulative process counters,
including work outside the input windows; unequal totals are not per-frame
workload-normalized comparisons.

The largest threshold-recorded geometry-unmap totals in blocks 0/1 are:
A1 10.1644/0.1036, B1 0.6363/0.2315, B2 2.1921/0.2596,
A2 0.9716/0.1869. The worst A1 operation spends 10.1632 ms in the direct
update stage. The large A1 event is absent in the repeated A2 control, so its
reduction must not be presented as a stable 10 ms saving.

## Combined device reuse and private heap

| Arm/block | Preparation median | Preparation p99 | Preparation max | Display max | Misses | Max motion error, physical px |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A1/0 | 0.77865 | 3.4880 | 10.8521 | 13.6138 | 4 | 97.972 |
| A1/1 | 0.71420 | 3.0695 | 8.8543 | 13.8924 | 7 | 114.710 |
| B1/0 | 0.73190 | 3.1322 | 5.0244 | 12.5090 | 3 | 106.901 |
| B1/1 | 0.70615 | 4.4623 | 13.7892 | 18.8095 | 6 | 104.895 |
| B2/0 | 0.74845 | 3.2156 | 5.9541 | 12.5411 | 2 | 75.498 |
| B2/1 | 0.69810 | 4.2103 | 8.2899 | 18.6902 | 6 | 105.666 |
| A2/0 | 0.83170 | 3.4982 | 5.9745 | 13.9481 | 6 | 98.423 |
| A2/1 | 0.79520 | 4.1532 | 6.7045 | 12.6028 | 6 | 104.105 |

B1/B2 each record one device creation and 23 hits, no device/reference errors,
and 481,033/543,802 private allocations. Four live blocks totaling 184 bytes
remain in each snapshot while the device cache is intentionally retained.
The combined correctness test verifies zero tracked live blocks after clearing
that cache. The capture counters are cumulative, not limited to active input.

The largest threshold-recorded unmap totals in blocks 0/1 are:
A1 4.7761/1.0864, B1 0.4972/0.6004, B2 0.2791/0.1437,
A2 0.3274/0.1536. Candidate direct-update maxima are below 0.5 ms.
The observer records only total operations reaching 100 µs. These maxima and
sample counts are not the full distribution; absence of a record is not zero
cost. No scheduler trace accompanies these comparison captures, so their
remaining gaps cannot be assigned to a particular lock or descheduling event.

Every block in both comparisons has zero coverage holds. Nevertheless, all
blocks fail the original motion criterion, and none of the four candidate
captures passes all timing/motion requirements. No gate is relaxed.

## Remaining preparation tail

Combined B1 block 1 has a 13.7892 ms preparation event. Its measured wall-time
components include:

- Before renderer body: 3.3145 ms.
- Scene render lists: 1.5937 ms.
- Batching: 1.2346 ms.
- Gate setup: 1.0115 ms.
- Geometry uploads: 3.6453 ms.
- Alpha-material preparation: 2.7071 ms.

The renderer records 138 added nodes in that event. This does not establish
that 138 new QML objects were allocated. Nearby loading counters report pending
bitmap work; a later 8.7205 ms event has a preceding pending count of zero.
Bitmap quiescence does not prove vector-source/scene-generation quiescence.
The cost is spread across phases. Without scheduling data, this is not proof
that all of that wall time is active CPU work.

Next, capture the combined candidate with scheduling/stack attribution and
split these preparation intervals into scheduled CPU, runnable delay and
blocked time. Attribute scene-list/upload/material tails before changing
another policy. Preserve all content, vector geometry, text fidelity, source
priority and alignment behavior. The cold, verified-warm-wheel and
quarter-height-middle acceptance gates remain outstanding.

## Evidence preservation

The device-only and combined analysis scripts run sequentially because the
motion analyzer uses a shared intermediate output. Per-case results are copied
immediately into the experiment directories. Sources, response files, logs,
capture identities, decoded-pixel hashes and screenshots are frozen, with
external binary/source hashes in the manifest.

Completed capture JSON/CSV files and inactive **correctness-test** executables
were NTFS-compressed to recover disk space. Before/after SHA256 checks prove
identical contents. Timed host executables were not compressed or altered.
No unique trace or binary was deleted.

AI assistance has been used to create this output.
